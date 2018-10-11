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
	"sync"
	"time"
)

type KubeClient interface {
	ListStrategies() ([]*vs.ValidationStrategy, error)
	ListRuns() ([]*vs.ValidationRun, error)
	ListPods(labels.Selector) ([]*core.Pod, error)
	CreateValidationRun(*vs.ValidationRun) error
	UpdateValidationRun(*vs.ValidationRun) error
	CreatePVC(*core.PersistentVolumeClaim) error
	CreateService(*core.Service) error
	CreateStatefulSet(*apps.StatefulSet) error
	CreateJob(*batch.Job) error
	GetPVC(string, string) (*core.PersistentVolumeClaim, error)
	GetJob(string, string) (*batch.Job, error)
}

type validator struct {
	kube  KubeClient
	mutex *sync.Mutex
}

type Validator interface {
	ProcessSnapshot(snapshot *snap.VolumeSnapshot) error
	ProcessValidationRun(run *vs.ValidationRun) error
}

func NewValidator(kube KubeClient) Validator {
	return &validator{
		kube,
		&sync.Mutex{},
	}
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

func (v *validator) matchStrategy(pod *core.Pod) (*vs.ValidationStrategy, error) {
	strategies, err := v.kube.ListStrategies()
	if err != nil {
		return nil, err
	}
	for _, s := range strategies {
		selector, err := meta.LabelSelectorAsSelector(&s.Spec.Selector)
		if err != nil {
			glog.Errorf("unable to create label selector %v/%v", s.Namespace, s.Name)
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

func initRun(strategy *vs.ValidationStrategy) *vs.ValidationRun {
	run := &vs.ValidationRun{
		Spec: vs.ValidationRunSpec{
			Snapshots: keys(strategy.Spec.Claims),
		},
		Status: vs.ValidationRunStatus{},
	}
	run.Spec.Claims = strategy.Spec.Claims
	//TODO: use incrementing counter instead of UUID
	//TODO: figure out "cannot use promoted field ObjectMeta.Name in struct literal of type alpha1.ValidationRun"
	run.Name = strategy.Name + "-" + string(uuid.NewUUID())
	run.Namespace = strategy.Namespace
	run.OwnerReferences = []meta.OwnerReference{{
		UID:        strategy.UID,
		APIVersion: "snapshotvalidator.ciscosso.io/v1alpha1",
		Kind:       "ValidationStrategy",
		Name:       "cassandra",
	}}

	glog.V(4).Infof("init run %v/%v from strategy - %#v", run.Namespace, run.Name, strategy)
	glog.V(4).Infof("init run %v/%v - %#v", run.Namespace, run.Name, run)
	return run
}

func (v *validator) getRun(snapshot *snap.VolumeSnapshot) (*vs.ValidationRun, bool, error) {
	pod, err := v.findPod(snapshot)
	if err != nil {
		return nil, false, err
	}
	strategy, err := v.matchStrategy(pod)
	if err != nil {
		return nil, false, err
	}
	run, err := v.matchRun(strategy)
	if err != nil {
		return nil, false, err
	}
	if run == nil {
		return initRun(strategy), true, nil
	}
	return run, false, nil
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

func (v *validator) createSnapshotPVCs(strategy *vs.ValidationStrategy, run *vs.ValidationRun) error {
	//TODO: refactor this so strategy doesn't contain Spec.Claims, only run.Spec.Claims
	for pvcName, pvc := range strategy.Spec.Claims {
		oldpvc, _ := v.kube.GetPVC(pvc.Namespace, pvc.Name)
		if oldpvc != nil {
			continue
		}
		snapshotName := run.Spec.Snapshots[pvcName]
		if pvc.Annotations == nil {
			pvc.Annotations = make(map[string]string)
		}
		pvc.Annotations["snapshot.alpha.kubernetes.io/snapshot"] = snapshotName
		pvc.OwnerReferences = []meta.OwnerReference{{
			UID:        run.UID,
			APIVersion: "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:       "ValidationRun",
			Name:       run.Name,
		}}
		err := v.kube.CreatePVC(&pvc)
		if err != nil {
			return e("creating PVC %v", err, pvc)
		}
	}
	if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) { return v.pvcsBound(run), nil }); err != nil {
		return e("run %v failed init, waiting for pvcs to bound", err, run)
	}
	return nil
}

func (v *validator) createJob(job *batch.Job, run *vs.ValidationRun) error {
	if job != nil {
		oldjob, _ := v.kube.GetJob(job.Namespace, job.Name)
		if oldjob != nil {
			return nil
		}
		job.OwnerReferences = []meta.OwnerReference{{
			UID:        run.UID,
			APIVersion: "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:       "ValidationRun",
			Name:       run.Name,
		}}
		if err := v.kube.CreateJob(job); err != nil {
			return e("creating job %v", err, job)
		}
		if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
			current, err := v.kube.GetJob(job.Namespace, job.Name)
			if err != nil {
				glog.Errorf("failed getting job %v/%v", job.Namespace, job.Name)
				return false, nil
			}
			return current.Status.Succeeded != 0, nil
		}); err != nil {
			return e("run %v failed waiting for job %v to finish", err, run, job)
		}
	}
	return nil
}

func (v *validator) createService(service *core.Service, run *vs.ValidationRun) error {
	if service != nil {
		service.OwnerReferences = []meta.OwnerReference{{
			UID:        run.UID,
			APIVersion: "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:       "ValidationRun",
			Name:       run.Name,
		}}
		if err := v.kube.CreateService(service); err != nil {
			return e("creating service %v", err, service)
		}
	}
	return nil
}
func (v *validator) createSts(sts *apps.StatefulSet, run *vs.ValidationRun) error {
	if sts != nil {
		sts.OwnerReferences = []meta.OwnerReference{{
			UID:        run.UID,
			APIVersion: "snapshotvalidator.ciscosso.io/v1alpha1",
			Kind:       "ValidationRun",
			Name:       run.Name,
		}}
		if err := v.kube.CreateStatefulSet(sts); err != nil {
			return e("creating sts %v", err, sts)
		}
		if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) { return v.podsReady(sts) }); err != nil {
			return e("waiting for pods to get ready %v", err, sts)
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
	for _, pvc := range run.Spec.Claims {
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
	if sts.Spec.Replicas == nil {
		glog.Errorf("replicas are nil for sts %v/%v", sts.Namespace, sts.Name)
		return false, nil
	}
	replicas := int(*sts.Spec.Replicas)
	if len(pods) != replicas {
		glog.V(4).Infof("replicas mismatch %v/%v %v!=%v", sts.Namespace, sts.Name, len(pods), replicas)
		return false, nil
	}
	for _, p := range pods {
		if p.Status.Phase != core.PodRunning {
			return false, nil
		}
	}
	return true, nil
}

func (v *validator) beforeInit(run *vs.ValidationRun) error {
	glog.Infof("before init run %v/%v", run.Namespace, run.Name)
	//TODO: validate the snapshots still exist
	for k, v := range run.Spec.Snapshots {
		if v == "" {
			glog.V(4).Infof("run %v/%v missing snapshot %v", run.Namespace, run.Name, k)
			return nil
		}
	}
	run.Status.InitStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) init(run *vs.ValidationRun) error {
	glog.Infof("init run %v/%v", run.Namespace, run.Name)
	strategy, err := v.getStrategy(run)
	if err != nil {
		return err
	}
	glog.Infof("create snapshot %v/%v PVCs", run.Namespace, run.Name)
	if err = v.createSnapshotPVCs(strategy, run); err != nil {
		return err
	}
	glog.Infof("create snapshot %v/%v service", run.Namespace, run.Name)
	if err = v.createService(strategy.Spec.Service, run); err != nil {
		return err
	}
	glog.Infof("create snapshot %v/%v sts", run.Namespace, run.Name)
	if err = v.createSts(strategy.Spec.StatefulSet, run); err != nil {
		return err
	}
	glog.Infof("create snapshot %v/%v init job", run.Namespace, run.Name)
	if err = v.createJob(strategy.Spec.Init, run); err != nil {
		return err
	}
	run.Status.InitFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) beforePreValidation(run *vs.ValidationRun) error {
	glog.Infof("before pre-validation run %v/%v", run.Namespace, run.Name)
	run.Status.PreValidationStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) preValidation(run *vs.ValidationRun) error {
	glog.Infof("pre-validation run %v/%v", run.Namespace, run.Name)
	strategy, err := v.getStrategy(run)
	if err != nil {
		return err
	}
	glog.Infof("create snapshot %v/%v pre-validation job", run.Namespace, run.Name)
	if err = v.createJob(strategy.Spec.PreValidation, run); err != nil {
		return err
	}
	run.Status.PreValidationFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}
func (v *validator) beforeValidation(run *vs.ValidationRun) error {
	glog.Infof("before validation run %v/%v", run.Namespace, run.Name)
	run.Status.ValidationStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}
func (v *validator) validation(run *vs.ValidationRun) error {
	glog.Infof("validation run %v/%v", run.Namespace, run.Name)
	strategy, err := v.getStrategy(run)
	if err != nil {
		return e("getting strategy for run %v", err, run)
	}
	glog.Infof("create snapshot %v/%v validation job", run.Namespace, run.Name)
	if err = v.createJob(strategy.Spec.Validation, run); err != nil {
		return e("creating job for run %v", err, run)
	}
	//TODO: annotate snapshots as valid
	run.Status.ValidationFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) tearDown(run *vs.ValidationRun) error {
	glog.V(4).Infof("teardown run %v/%v", run.Namespace, run.Name)
	//TODO: implement this
	return nil
}
func (v *validator) ProcessValidationRun(run *vs.ValidationRun) (err error) {
	//TODO: add mutex per validation strategy
	glog.Infof("processing validation run %v/%v", run.Namespace, run.Name)
	v.mutex.Lock()
	defer v.mutex.Unlock()

	//TODO: do deep copy only when necessary
	copy := run.DeepCopy()
	if run.Status.InitStarted == nil {
		err = v.beforeInit(copy)
	} else if run.Status.InitFinished == nil {
		err = v.init(copy)
	} else if run.Status.PreValidationStarted == nil {
		err = v.beforePreValidation(copy)
	} else if run.Status.PreValidationFinished == nil {
		err = v.preValidation(copy)
	} else if run.Status.ValidationStarted == nil {
		err = v.beforeValidation(copy)
	} else if run.Status.ValidationFinished == nil {
		err = v.validation(copy)
	} else {
		err = v.tearDown(copy)
	}

	if err != nil {
		err = e("processing validator run %v failed", err, run)
	} else {
		glog.Infof("finished processing validator run %v/%v", run.Namespace, run.Name)
	}
	return
}

func (v *validator) updateRun(run *vs.ValidationRun, snapshot *snap.VolumeSnapshot, new bool) error {
	pvc := snapshot.Spec.PersistentVolumeClaimName
	if _, ok := run.Spec.Snapshots[pvc]; !ok {
		return fmt.Errorf("ValidationRun %v/%v doesn't contain PVC %v", run.Namespace, run.Name, pvc)
	}
	run.Spec.Snapshots[pvc] = snapshot.Metadata.Name
	if new {
		return e("Creating run %v", v.kube.CreateValidationRun(run), run)
	} else {
		return e("Updating run %v", v.kube.UpdateValidationRun(run), run)
	}
}

func (v *validator) ProcessSnapshot(snapshot *snap.VolumeSnapshot) (err error) {
	//TODO: add mutex per validation strategy
	glog.Infof("processing snapshot %v/%v", snapshot.Metadata.Namespace, snapshot.Metadata.Name)
	v.mutex.Lock()
	defer v.mutex.Unlock()
	run, new, err := v.getRun(snapshot)
	if run != nil {
		glog.V(4).Infof("got run %v/%v for snapshot %v/%v, new(%v)", run.Namespace, run.Name, snapshot.Metadata.Namespace, snapshot.Metadata.Name, new)
	} else {
		glog.V(4).Infof("run is nil for snapshot %v/%v", snapshot.Metadata.Namespace, snapshot.Metadata.Name)
	}
	if err != nil {
		err = e("processing snapshot %v failed getting run", err, snapshot)
		return
	}
	err = v.updateRun(run, snapshot, new)
	if err != nil {
		err = e("processing snapshot %v failed updating run", err, snapshot)
		return
	}
	glog.Infof("finished processing snapshot %v/%v successfully", snapshot.Metadata.Namespace, snapshot.Metadata.Name)
	return
}

func e(msg string, err error, obj ...interface{}) error {
	if err == nil {
		return nil
	}
	ks := make([]interface{}, 0)
	for _, o := range obj {
		k, err := kcache.MetaNamespaceKeyFunc(o)
		if err != nil {
			glog.Errorf("Object has no metadata %#v", o)
			ks = append(ks, o)
		} else {
			ks = append(ks, k)
		}

	}
	nmsg := fmt.Sprintf(msg, ks...)
	return fmt.Errorf(nmsg+": %v", err)
}
