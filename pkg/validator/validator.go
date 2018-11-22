package validator

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
	"time"
)

var (
	// treat as constants
	block = true
	zero  = 0
)

type validator struct {
	kube  KubeCalls
	mutex map[string]*sync.Mutex
}

type Validator interface {
	ProcessSnapshot(snapshot *snap.VolumeSnapshot) error
	ProcessValidationRun(run *vs.ValidationRun) error
}

func NewValidator(kube KubeCalls) Validator {
	return &validator{
		kube,
		make(map[string]*sync.Mutex),
	}
}

func (v *validator) beforeKust(run *vs.ValidationRun) error {
	glog.V(2).Infof("before kust run %v/%v", run.Namespace, run.Name)
	for k, v := range run.Spec.ClaimsToSnapshots {
		if v == "" {
			return fmt.Errorf("claim %v is missing snapshot", k)
		}
	}
	run.Status.KustStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) kust(run *vs.ValidationRun) error {
	glog.V(2).Infof("kustomize run %v/%v", run.Namespace, run.Name)
	strategy, err := v.getStrategy(run)
	if err != nil {
		return e("failed getting strategy", err)
	}
	if err = v.kustomize(strategy, run); err != nil {
		return e("failed kustomize", err)
	}
	run.Status.KustFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) beforeInit(run *vs.ValidationRun) error {
	glog.V(2).Infof("before init run %v/%v", run.Namespace, run.Name)
	run.Status.InitStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) init(run *vs.ValidationRun) error {
	glog.V(2).Infof("init run %v/%v", run.Namespace, run.Name)
	strategy, err := v.getStrategy(run)
	if err != nil {
		return e("failed getting strategy", err)
	}
	glog.V(4).Infof("create snapshot PVCs for run %v/%v", run.Namespace, run.Name)
	if err = v.createSnapshotPVCs(run); err != nil {
		return e("failed to create snapshot PVCs", err)
	}
	glog.V(4).Infof("create snapshot objects for run %v/%v", run.Namespace, run.Name)
	if err = v.createObjects(run); err != nil {
		return e("failed to create kustomized objects", err)
	}
	glog.V(4).Infof("create snapshot init job for run %v/%v", run.Namespace, run.Name)
	if err = v.createJob("init", strategy.Spec.Hooks.Init, run); err != nil {
		return e("failed to create init job", err)
	}
	run.Status.InitFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) beforePreValidation(run *vs.ValidationRun) error {
	glog.V(2).Infof("before pre-validation run %v/%v", run.Namespace, run.Name)
	run.Status.PreValidationStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) preValidation(run *vs.ValidationRun) error {
	glog.V(2).Infof("pre-validation run %v/%v", run.Namespace, run.Name)
	strategy, err := v.getStrategy(run)
	if err != nil {
		return err
	}
	glog.V(4).Infof("create snapshot %v/%v pre-validation job", run.Namespace, run.Name)
	if err = v.createJob("prevalidation", strategy.Spec.Hooks.PreValidation, run); err != nil {
		return err
	}
	run.Status.PreValidationFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) beforeValidation(run *vs.ValidationRun) error {
	glog.V(2).Infof("before validation run %v/%v", run.Namespace, run.Name)
	run.Status.ValidationStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) validation(run *vs.ValidationRun) error {
	glog.V(2).Infof("validation run %v/%v", run.Namespace, run.Name)
	strategy, err := v.getStrategy(run)
	if err != nil {
		return e("getting strategy for run %v", err, run)
	}
	glog.V(4).Infof("create snapshot %v/%v validation job", run.Namespace, run.Name)
	if err = v.createJob("validation", strategy.Spec.Hooks.Validation, run); err != nil {
		return e("creating job for run %v", err, run)
	}
	for claim, snapshot := range run.Spec.ClaimsToSnapshots {
		s, err := v.kube.GetSnapshot(run.Namespace, snapshot)
		if err != nil {
			return e("getting snapshot %v for run %v", err, snapshot, run)
		}
		pvc, err := v.kube.GetPVC(run.Namespace, claim)
		if err != nil {
			return e("getting PVC %v for run %v", err, claim, run)
		}
		pv, err := v.kube.GetPV(pvc.Spec.VolumeName)
		if err != nil {
			return e("getting PV %v for PVC %v and run %v", err, pvc.Spec.VolumeName, claim, run)
		}
		label, id, err := getId(pv)
		if err != nil {
			return fmt.Errorf("failed getting volume id for PV %v, PVC %v and run %v/%v", pv.Name, pvc.Name, run.Namespace, run.Name)
		}
		if err = v.kube.LabelSnapshot(s.Metadata.Namespace, s.Metadata.Name, label, id); err != nil {
			return e("labeling snapshot %v for run %v", err, s, run)
		}
	}
	run.Status.ValidationFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) beforeCleanup(run *vs.ValidationRun) error {
	glog.V(2).Infof("before cleanup run %v/%v", run.Namespace, run.Name)
	run.Status.CleanupStarted = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) cleanup(run *vs.ValidationRun) error {
	glog.V(2).Infof("cleanup run %v/%v", run.Namespace, run.Name)
	if !run.Spec.Cleanup {
		glog.V(2).Infof("cleanup run %v/%v paused", run.Namespace, run.Name)
		return nil
	}
	v.kube.DeleteValidationRun(run)
	for _, claim := range run.Spec.Objects.Claims {
		v.kube.DeletePVC(&claim)
	}
	run.Status.CleanupFinished = &meta.Time{time.Now()}
	return v.kube.UpdateValidationRun(run)
}

func (v *validator) ProcessValidationRun(run *vs.ValidationRun) (err error) {
	glog.Infof("processing validation run %v/%v", run.Namespace, run.Name)
	mutexId := run.Namespace + "/" + run.Name
	if v.mutex[mutexId] == nil {
		v.mutex[mutexId] = &sync.Mutex{}
	}
	v.mutex[mutexId].Lock()
	defer v.mutex[mutexId].Unlock()

	copy := run.DeepCopy()
	if run.Status.KustStarted == nil {
		err = v.beforeKust(copy)
	} else if run.Status.KustFinished == nil {
		err = v.kust(copy)
	} else if run.Status.InitStarted == nil {
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
	} else if run.Status.CleanupStarted == nil {
		err = v.beforeCleanup(copy)
	} else if run.Status.CleanupFinished == nil {
		err = v.cleanup(copy)
	}

	if err != nil {
		err = e("Error processing validator run %v", err, run)
	} else {
		glog.V(2).Infof("finished processing validator run %v/%v", run.Namespace, run.Name)
	}
	return
}

func (v *validator) ProcessSnapshot(snapshot *snap.VolumeSnapshot) error {
	glog.Infof("processing snapshot %v/%v", snapshot.Metadata.Namespace, snapshot.Metadata.Name)
	run, strategy, new, err := v.getRun(snapshot)
	if run != nil {
		glog.V(4).Infof("got run %v/%v for snapshot %v/%v, new(%v)", run.Namespace, run.Name, snapshot.Metadata.Namespace, snapshot.Metadata.Name, new)
	} else {
		glog.V(4).Infof("run is nil for snapshot %v/%v", snapshot.Metadata.Namespace, snapshot.Metadata.Name)
	}
	if err != nil {
		return e("processing snapshot %v failed getting run", err, snapshot)
	}
	mutexId := strategy.Namespace + "/" + strategy.Name
	if v.mutex[mutexId] == nil {
		v.mutex[mutexId] = &sync.Mutex{}
	}
	v.mutex[mutexId].Lock()
	defer v.mutex[mutexId].Unlock()
	err = v.updateRun(run, snapshot, new)
	if err != nil {
		return e("processing snapshot %v failed updating run", err, snapshot)
	}
	glog.Infof("finished processing snapshot %v/%v successfully", snapshot.Metadata.Namespace, snapshot.Metadata.Name)
	return nil
}
