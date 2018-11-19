package validator

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	kcache "k8s.io/client-go/tools/cache"
	"strings"
	"time"
)

func (v *validator) updateRun(run *vs.ValidationRun, snapshot *snap.VolumeSnapshot, new bool) error {
	pvc := snapshot.Spec.PersistentVolumeClaimName
	if _, ok := run.Spec.ClaimsToSnapshots[pvc]; !ok {
		return fmt.Errorf("ValidationRun %v/%v doesn't contain PVC %v", run.Namespace, run.Name, pvc)
	}
	run.Spec.ClaimsToSnapshots[pvc] = snapshot.Metadata.Name
	if new {
		return e("Creating run %v", v.kube.CreateValidationRun(run), run)
	} else {
		return e("Updating run %v", v.kube.UpdateValidationRun(run), run)
	}
}

func e(msg string, err error, obj ...interface{}) error {
	if err == nil {
		return nil
	}
	ks := make([]interface{}, 0)
	for _, o := range obj {
		switch o.(type) {
		case string:
			ks = append(ks, o)
		default:
			k, err := kcache.MetaNamespaceKeyFunc(o)
			if err != nil {
				glog.Errorf("Object has no metadata %#v", o)
				ks = append(ks, o)
			} else {
				ks = append(ks, k)
			}
		}
	}
	nmsg := fmt.Sprintf(msg, ks...)
	return fmt.Errorf(nmsg+": %v", err)
}

func (v *validator) findPod(snapshot *snap.VolumeSnapshot) (*core.Pod, error) {
	//TODO: implement as InformerIndexer
	pods, err := v.kube.ListPods(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, p := range pods {
		if p.Namespace != snapshot.Metadata.Namespace {
			continue
		}
		for _, v := range p.Spec.Volumes {
			if v.PersistentVolumeClaim == nil {
				continue
			}
			pvc := v.PersistentVolumeClaim.ClaimName
			spvc := snapshot.Spec.PersistentVolumeClaimName
			glog.V(4).Infof("Comparing pod %v/%v - volume %v to snapshot pvc %v", p.Namespace, p.Name, pvc, spvc)
			if pvc == spvc {
				return p, nil
			}
		}
	}
	return nil, fmt.Errorf("No pod for snapshot %v/%v found.", snapshot.Metadata.Namespace, snapshot.Metadata.Name)
}

func (v *validator) getLabelSelector(strategy *vs.ValidationStrategy) (labels.Selector, error) {
	if strategy.Spec.StsType != nil {
		sts, err := v.kube.GetSts(strategy.Namespace, strategy.Spec.StsType.Name)
		if err != nil {
			return nil, e("getting sts %v for labelSelector", err, strategy.Spec.StsType.Name)
		}
		selector, err := meta.LabelSelectorAsSelector(sts.Spec.Selector)
		if err != nil {
			return nil, e("unable to create label selector for strategy %v", err, strategy)
		}
		return selector, nil
	}
	return nil, fmt.Errorf("unknown strategy type")
}

func (v *validator) matchStrategy(pod *core.Pod) (*vs.ValidationStrategy, error) {
	strategies, err := v.kube.ListStrategies()
	if err != nil {
		return nil, err
	}
	for _, s := range strategies {
		selector, err := v.getLabelSelector(s)
		if err != nil {
			glog.Errorf(err.Error())
			continue
		}
		if selector.Matches(labels.Set(pod.Labels)) {
			return s, nil
		}
	}
	return nil, fmt.Errorf("Strategy for pod %v/%v not found", pod.Namespace, pod.Name)
}

func (v *validator) matchRun(strategy *vs.ValidationStrategy) (*vs.ValidationRun, error) {
	//TODO: implement as InformerIndexer
	//TODO: resync
	runs, err := v.kube.ListRuns()
	if err != nil {
		return nil, err
	}
	glog.V(4).Infof("matching run for strategy %v/%v, from %v runs", strategy.Namespace, strategy.Name, len(runs))
	//TODO: sort runs by time
	for _, r := range runs {
		if r.Status.InitStarted != nil {
			continue
		}
		for _, ref := range r.OwnerReferences {
			if ref.UID == strategy.UID {
				return r.DeepCopy(), nil
			}
		}
	}
	return nil, nil
}

func keys(keys map[string]core.PersistentVolumeClaim) map[string]string {
	m := make(map[string]string)
	for k, _ := range keys {
		m[k] = ""
	}
	return m
}

func (v *validator) getPVCsMap(strategy *vs.ValidationStrategy) (map[string]string, error) {
	m := make(map[string]string)
	selector, err := v.getLabelSelector(strategy)
	if err != nil {
		return m, err
	}
	pvcs, err := v.kube.ListPVCs(selector)
	if err != nil {
		return m, e("Listing PVCs using selector %v", err, selector)
	}
	for _, pvc := range pvcs {
		m[pvc.Name] = ""
	}
	return m, nil
}

func (v *validator) initRun(strategy *vs.ValidationStrategy) (*vs.ValidationRun, error) {
	pvcs, err := v.getPVCsMap(strategy)
	if err != nil {
		return nil, e("getting snapshot map for strategy %v", err, strategy)
	}
	run := &vs.ValidationRun{
		Spec: vs.ValidationRunSpec{
			ClaimsToSnapshots: pvcs,
		},
		Status: vs.ValidationRunStatus{},
	}
	//TODO: use incrementing counter instead of UUID
	run.Spec.Suffix = string(uuid.NewUUID())
	run.Name = strategy.Name + "-" + run.Spec.Suffix
	run.Namespace = strategy.Namespace
	run.OwnerReferences = []meta.OwnerReference{{
		UID:                strategy.UID,
		APIVersion:         "snapshotvalidator.ciscosso.io/v1alpha1",
		Kind:               "ValidationStrategy",
		Name:               "cassandra",
		BlockOwnerDeletion: &block,
	}}
	glog.V(4).Infof("init run %v/%v from strategy - %#v", run.Namespace, run.Name, strategy)
	glog.V(4).Infof("init run %v/%v - %#v", run.Namespace, run.Name, run)
	return run, nil
}

func (v *validator) getRun(snapshot *snap.VolumeSnapshot) (*vs.ValidationRun, bool, error) {
	pod, err := v.findPod(snapshot)
	new, failed := true, false
	if err != nil {
		return nil, failed, err
	}
	strategy, err := v.matchStrategy(pod)
	if err != nil {
		return nil, failed, err
	}
	run, err := v.matchRun(strategy)
	if err != nil {
		return nil, failed, err
	}
	if run == nil {
		run, err = v.initRun(strategy)
		if err != nil {
			return nil, failed, err
		}
		return run, new, nil
	}
	return run, !new, nil
}

func (v *validator) getStrategy(run *vs.ValidationRun) (*vs.ValidationStrategy, error) {
	strategies, err := v.kube.ListStrategies()
	if err != nil {
		return nil, err
	}
	for _, s := range strategies {
		for _, ref := range run.OwnerReferences {
			if s.UID == ref.UID {
				return s, nil
			}
		}
	}
	return nil, fmt.Errorf("Unable to find strategy for run %v/%v", run.Namespace, run.Name)
}

func (v *validator) createSnapshotPVCs(run *vs.ValidationRun) error {
	for _, pvc := range run.Spec.Objects.Claims {
		oldpvc, _ := v.kube.GetPVC(pvc.Namespace, pvc.Name)
		if oldpvc != nil {
			continue
		}
		snapshotName := run.Spec.ClaimsToSnapshots[pvc.Name]
		if pvc.Annotations == nil {
			pvc.Annotations = make(map[string]string)
		}
		pvc.Annotations["snapshot.alpha.kubernetes.io/snapshot"] = snapshotName
		pvc.OwnerReferences = []meta.OwnerReference{{
			UID:                run.UID,
			APIVersion:         "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:               "ValidationRun",
			Name:               run.Name,
			BlockOwnerDeletion: &block,
		}}
		err := v.kube.CreatePVC(&pvc)
		if err != nil {
			return e("creating PVC %v", err, pvc)
		}
	}
	if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
		allBound := v.pvcsBound(run)
		if !allBound {
			glog.Infof("PVCs not bound for run %v/%v", run.Namespace, run.Name)
		}
		return allBound, nil
	}); err != nil {
		return e("run %v failed init, waiting for pvcs to bound", err, run)
	}
	return nil
}

func (v *validator) createJob(name string, jobSpec *batch.JobSpec, run *vs.ValidationRun) error {
	name = run.Name + name
	if jobSpec != nil {
		oldjob, _ := v.kube.GetJob(run.Namespace, name)
		if oldjob != nil {
			return nil
		}
		job := batch.Job{}
		job.OwnerReferences = []meta.OwnerReference{{
			UID:                run.UID,
			APIVersion:         "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:               "ValidationRun",
			Name:               run.Name,
			BlockOwnerDeletion: &block,
		}}
		job.Name = name
		job.Namespace = run.Namespace
		job.Spec = *jobSpec
		if err := v.kube.CreateJob(&job); err != nil {
			return e("creating job %v", err, job)
		}
		if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
			current, err := v.kube.GetJob(job.Namespace, job.Name)
			if err != nil {
				glog.Errorf("polling - failed getting job %v/%v", job.Namespace, job.Name)
				return false, nil
			}
			return current.Status.Succeeded > 0, nil
		}); err != nil {
			return e("run %v failed waiting for job %v to finish", err, run, job)
		}
	}
	return nil
}

func (v *validator) createService(service *core.Service, run *vs.ValidationRun) error {
	if service != nil {
		oldsvc, _ := v.kube.GetService(service.Namespace, service.Name)
		if oldsvc != nil {
			return nil
		}
		service.OwnerReferences = []meta.OwnerReference{{
			UID:                run.UID,
			APIVersion:         "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:               "ValidationRun",
			Name:               run.Name,
			BlockOwnerDeletion: &block,
		}}
		if err := v.kube.CreateService(service); err != nil {
			return e("creating service %v", err, service)
		}
	}
	return nil
}

func (v *validator) createSts(sts *apps.StatefulSet, run *vs.ValidationRun) error {
	if sts != nil {
		oldsts, _ := v.kube.GetSts(sts.Namespace, sts.Name)
		if oldsts != nil {
			return nil
		}
		sts.OwnerReferences = []meta.OwnerReference{{
			UID:                run.UID,
			APIVersion:         "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:               "ValidationRun",
			Name:               run.Name,
			BlockOwnerDeletion: &block,
		}}
		if err := v.kube.CreateStatefulSet(sts); err != nil {
			return e("creating sts %v", err, sts)
		}
		if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) { return v.podsReady(sts) }); err != nil {
			return e("waiting for pods ready %v", err, sts)
		}
	}
	return nil
}

func allTrue(m map[string]bool) bool {
	for _, k := range m {
		if !k {
			return false
		}
	}
	return true
}

func (v *validator) pvcsBound(run *vs.ValidationRun) bool {
	for _, pvc := range run.Spec.Objects.Claims {
		current, err := v.kube.GetPVC(pvc.Namespace, pvc.Name)
		if err != nil {
			return false
		}
		if current.Status.Phase != core.ClaimBound {
			return false
		}
	}
	return true
}

func getId(pv *core.PersistentVolume) (string, string, error) {
	//TODO add other volume sources
	if pv.Spec.Cinder != nil {
		return "validated-cinder-volume", pv.Spec.Cinder.VolumeID, nil
	}
	return "unknown", "unknown", fmt.Errorf("TODO: implement getId for other than cinder source")
}

func (v *validator) createObjects(obj []string) error {
	var errors []string
	for _, o := range obj {
		if err := v.kube.CreateObjectYAML(o); err != nil {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) != 0 {
		return fmt.Errorf("Unable to create objects %v", strings.Join(errors, ", "))
	}
	return nil
}

func (v *validator) podsReady(sts *apps.StatefulSet) (bool, error) {
	selector, err := meta.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return false, err
	}
	pods, err := v.kube.ListPods(selector)
	if err != nil {
		glog.Errorf("listing pods for sts %v/%v", sts.Namespace, sts.Name)
		return false, nil
	}
	var replicas int
	if sts.Spec.Replicas == nil {
		replicas = 1
	} else {
		replicas = int(*sts.Spec.Replicas)
	}
	if len(pods) != replicas {
		glog.V(4).Infof("  replicas mismatch %v/%v %v!=%v", sts.Namespace, sts.Name, len(pods), replicas)
		return false, nil
	}
	for _, p := range pods {
		if p.Status.Phase != core.PodRunning {
			return false, nil
		}
	}
	return true, nil
}