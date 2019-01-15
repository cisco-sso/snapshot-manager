package manager

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotmanager/v1alpha1"
	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	kcache "k8s.io/client-go/tools/cache"
	"regexp"
	"strings"
	"time"
)

var (
	sanitizeLabelRe = regexp.MustCompile("[^A-Za-z0-9]+")
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
	run.Spec.Suffix = string(uuid.NewUUID())
	run.Name = strategy.Name + "-" + run.Spec.Suffix
	run.Namespace = strategy.Namespace
	run.OwnerReferences = []meta.OwnerReference{{
		UID:                strategy.UID,
		APIVersion:         "snapshotmanager.ciscosso.io/v1alpha1",
		Kind:               "ValidationStrategy",
		Name:               strategy.Name,
		BlockOwnerDeletion: &block,
	}}
	glog.V(4).Infof("init run %v/%v from strategy - %#v", run.Namespace, run.Name, strategy)
	glog.V(4).Infof("init run %v/%v - %#v", run.Namespace, run.Name, run)
	return run, nil
}

func (v *validator) getStrategyForSnapshot(snapshot *snap.VolumeSnapshot) (*vs.ValidationStrategy, error) {
	pod, err := v.findPod(snapshot)
	if err != nil {
		return nil, err
	}
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

func (v *validator) getRunForStrategy(strategy *vs.ValidationStrategy) (*vs.ValidationRun, bool, error) {
	new, failed := true, false
	run, err := v.kube.MatchRun(strategy)
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

func (v *validator) getStrategyForRun(run *vs.ValidationRun) (*vs.ValidationStrategy, error) {
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
		pvc.OwnerReferences = []meta.OwnerReference{{
			UID:                run.UID,
			APIVersion:         "snapshotmanager.ciscosso.io/v1alpha1",
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
		allBound := v.kube.PvcsBound(pvcToPtr(run.Spec.Objects.Claims))
		if !allBound {
			glog.Infof("PVCs not bound for run %v/%v", run.Namespace, run.Name)
		}
		return allBound, nil
	}); err != nil {
		return e("run %v failed init, waiting for pvcs to bound", err, run)
	}
	return nil
}

func pvcToPtr(pvcs []core.PersistentVolumeClaim) []*core.PersistentVolumeClaim {
	c := make([]*core.PersistentVolumeClaim, 0, len(pvcs))
	for _, pvc := range pvcs {
		c = append(c, &pvc)
	}
	return c
}

func (v *validator) createJob(name string, jobSpec *batch.JobSpec, run *vs.ValidationRun) error {
	name = name + "-" + run.Name
	if jobSpec != nil {
		oldjob, _ := v.kube.GetJob(run.Namespace, name)
		var job batch.Job
		if oldjob != nil {
			job = *oldjob
		} else {
			job.OwnerReferences = []meta.OwnerReference{{
				UID:                run.UID,
				APIVersion:         "snapshotmanager.ciscosso.io/v1alpha1",
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
			APIVersion:         "snapshotmanager.ciscosso.io/v1alpha1",
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
			APIVersion:         "snapshotmanager.ciscosso.io/v1alpha1",
			Kind:               "ValidationRun",
			Name:               run.Name,
			BlockOwnerDeletion: &block,
		}}
		if err := v.kube.CreateStatefulSet(sts); err != nil {
			return e("creating sts %v", err, sts)
		}
		if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) { return v.kube.PodsReady(sts) }); err != nil {
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

func sanitizeLabel(pvid string) string {
	return sanitizeLabelRe.ReplaceAllString(pvid, "-")
}

func getId(pv *core.PersistentVolume) (string, string, error) {
	//TODO add other volume sources
	if pv.Spec.Cinder != nil {
		return "validated-cinder-volume", pv.Spec.Cinder.VolumeID, nil
	} else if pv.Spec.GCEPersistentDisk != nil {
		return "validated-GCEPersistentDisk-volume", pv.Spec.GCEPersistentDisk.PDName, nil
	} else if pv.Spec.AWSElasticBlockStore != nil {
		return "validated-AWSElasticBlockStore-volume", pv.Spec.AWSElasticBlockStore.VolumeID, nil
	} else if pv.Spec.HostPath != nil {
		return "validated-HostPath-volume", pv.Spec.HostPath.Path, nil
	} else if pv.Spec.Glusterfs != nil {
		return "validated-Glusterfs-volume", pv.Spec.Glusterfs.EndpointsName, nil
	} else if pv.Spec.NFS != nil {
		return "validated-NFS-volume", pv.Spec.NFS.Server + ":" + pv.Spec.NFS.Path, nil
	} else if pv.Spec.RBD != nil {
		return "validated-RBD-volume", pv.Spec.RBD.RBDImage, nil
	} else if pv.Spec.ISCSI != nil {
		return "validated-ISCSI-volume", pv.Spec.ISCSI.IQN, nil
	} else if pv.Spec.CephFS != nil {
		return "validated-CephFS-volume", pv.Spec.CephFS.Path, nil
	} else if pv.Spec.Flocker != nil {
		return "validated-Flocker-volume", pv.Spec.Flocker.DatasetUUID, nil
	} else if pv.Spec.AzureFile != nil {
		return "validated-AzureFile-volume", pv.Spec.AzureFile.ShareName, nil
	} else if pv.Spec.VsphereVolume != nil {
		return "validated-VsphereVolume-volume", pv.Spec.VsphereVolume.VolumePath, nil
	} else if pv.Spec.Quobyte != nil {
		return "validated-Quobyte-volume", pv.Spec.Quobyte.Volume, nil
	} else if pv.Spec.AzureDisk != nil {
		return "validated-AzureDisk-volume", pv.Spec.AzureDisk.DiskName, nil
	} else if pv.Spec.PhotonPersistentDisk != nil {
		return "validated-PhotonPersistentDisk-volume", pv.Spec.PhotonPersistentDisk.PdID, nil
	} else if pv.Spec.PortworxVolume != nil {
		return "validated-PortworxVolume-volume", pv.Spec.PortworxVolume.VolumeID, nil
	} else if pv.Spec.Local != nil {
		return "validated-Local-volume", pv.Spec.Local.Path, nil
	} else if pv.Spec.StorageOS != nil {
		return "validated-StorageOS-volume", pv.Spec.StorageOS.VolumeName, nil
	} else if pv.Spec.CSI != nil {
		return "validated-CSI-volume", pv.Spec.CSI.VolumeHandle, nil
	}
	return "unknown", "unknown", fmt.Errorf("TODO: implement getId for other source")
}

func (v *validator) createObjects(run *vs.ValidationRun) error {
	var errors []string
	for i, o := range run.Spec.Objects.Kustomized {
		u := unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(o), &u); err != nil {
			errors = append(errors, e("Unmarshal yaml %d. %v", err, i, o).Error())
			continue
		}
		u.SetOwnerReferences([]meta.OwnerReference{{
			UID:                run.UID,
			APIVersion:         "snapshotmanager.ciscosso.io/v1alpha1",
			Kind:               "ValidationRun",
			Name:               run.Name,
			BlockOwnerDeletion: &block,
		}})
		if err := v.kube.CreateUnstructuredObject(u); err != nil {
			errors = append(errors, e("Create object from unstructured %d. %v", err, i, u).Error())
		}
	}
	if len(errors) != 0 {
		return fmt.Errorf("Unable to create objects [%v]", strings.Join(errors, ", "))
	}
	return nil
}
