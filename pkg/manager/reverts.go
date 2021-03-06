package manager

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotmanager/v1alpha1"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"
)

type reverts struct {
	kube KubeCalls
}

type Reverts interface {
	ProcessSnapshotRevert(snapshotRevert *vs.SnapshotRevert) error
}

func NewReverts(kube KubeCalls) Reverts {
	return &reverts{
		kube,
	}
}

func (r *reverts) init(revert *vs.SnapshotRevert) error {
	glog.V(6).Infof("init snapshot revert %v/%v", revert.Namespace, revert.Name)
	sts, err := r.kube.GetSts(revert.Namespace, revert.Spec.StsType.Name)
	if err != nil {
		return e("Unable to get STS for %v revert", err, revert)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas == 0 {
		return fmt.Errorf("STS for %v revert has %v replicas", revert, sts.Spec.Replicas)
	}
	selector, err := meta.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return e("Unable to create label selector for revert %v", err, revert)
	}
	pvcs, err := r.kube.ListPVCs(selector)
	if err != nil {
		return e("Unable to list PVCs for revert %v", err, revert)
	}
	details := &revert.Status.Reverts[len(revert.Status.Reverts)-1]
	details.Replicas = sts.Spec.Replicas
	details.OldClaims = pvcs
	setState(revert, "init")
	return nil
}

func initDetails(revert *vs.SnapshotRevert) *vs.SnapshotRevertDetails {
	glog.V(6).Infof("init details for snapshot revert %v/%v", revert.Namespace, revert.Name)
	if len(revert.Status.Reverts) == 0 {
		revert.Status.Reverts = append(revert.Status.Reverts, vs.SnapshotRevertDetails{})
	}
	if revert.Status.Reverts[len(revert.Status.Reverts)-1].State == "finished" {
		revert.Status.Reverts = append(revert.Status.Reverts, vs.SnapshotRevertDetails{})
	}
	return &revert.Status.Reverts[len(revert.Status.Reverts)-1]
}

func (r *reverts) pause(revert *vs.SnapshotRevert) error {
	glog.V(6).Infof("pause snapshot revert %v/%v", revert.Namespace, revert.Name)
	sts, err := r.kube.GetSts(revert.Namespace, revert.Spec.StsType.Name)
	if err != nil {
		return e("Unable to get STS for %v revert", err, revert)
	}
	if sts.Spec.Replicas != nil && *sts.Spec.Replicas != 0 {
		err = r.kube.SetStsReplica(sts.Namespace, sts.Name, 0)
		if err != nil {
			return e("Failed to scale down sts for revert %v", err, revert)
		}
	}
	if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) { return r.kube.PodsDeleted(sts) }); err != nil {
		return e("waiting for pods deleted %v", err, sts)
	}
	setState(revert, "paused")
	return nil
}

func (r *reverts) revertSnapshots(revert *vs.SnapshotRevert) error {
	glog.V(6).Infof("revertSnapshots snapshot revert %v/%v", revert.Namespace, revert.Name)
	details := &revert.Status.Reverts[len(revert.Status.Reverts)-1]
	snapshots, err := r.getSnapshots(details.OldClaims, revert.Spec.Action.FromTime, revert.Spec.Action.ToTime)
	if err != nil {
		return e("Unable to find required matching snapshots for revert %v", err, revert)
	}
	for _, pvc := range details.OldClaims {
		pv, err := r.kube.GetPV(pvc.Spec.VolumeName)
		if err != nil {
			return e("Failed to get pv for pvc %v for revert %v", err, pvc, revert)
		}
		err = r.fixReclaimPolicy(pv)
		if err != nil {
			return err
		}
		err = r.kube.DeletePVC(pvc)
		if err != nil {
			return e("Failed to delete pvc %v for revert %v", err, pvc, revert)
		}
		pv, err = r.kube.GetPV(pvc.Spec.VolumeName)
		if err != nil {
			return e("Failed to get pv for pvc %v for revert %v", err, pvc, revert)
		}
		err = r.availablePV(pv)
		if err != nil {
			return err
		}
	}
	for _, pvc := range details.OldClaims {
		snapshotPvc := revert.AttachSnapshot(pvc, snapshots[pvc.Name].Metadata.Name)
		err := r.kube.CreatePVC(snapshotPvc)
		if err != nil {
			return e("Failed to create snapshot pvc %v for revert %v", err, snapshotPvc, revert)
		}
	}
	if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
		return r.kube.PvcsBound(details.OldClaims), nil
	}); err != nil {
		return e("revert %v failed init, waiting for pvcs to bound", err, revert)
	}
	setState(revert, "reverted")
	return nil
}

func undo(pvc *core.PersistentVolumeClaim) *core.PersistentVolumeClaim {
	new := &core.PersistentVolumeClaim{}
	new.Name = pvc.Name
	new.Namespace = pvc.Namespace
	new.Labels = pvc.Labels
	new.Spec = pvc.Spec
	return new
}

func (r *reverts) undoRevertSnapshots(revert *vs.SnapshotRevert) error {
	glog.V(6).Infof("undoRevertSnapshots snapshot revert %v/%v", revert.Namespace, revert.Name)
	details := &revert.Status.Reverts[len(revert.Status.Reverts)-1]
	for _, pvc := range details.OldClaims {
		err := r.kube.DeletePVC(pvc)
		if err != nil {
			return e("Failed to delete pvc %v for revert undo %v", err, pvc, revert)
		}
	}
	for _, pvc := range details.OldClaims {
		err := r.kube.CreatePVC(undo(pvc))
		if err != nil {
			return e("Failed to create pvc %v for revert undo %v", err, pvc, revert)
		}
	}
	if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) {
		return r.kube.PvcsBound(details.OldClaims), nil
	}); err != nil {
		return e("Undo revert %v failed init, waiting for pvcs to bound", err, revert)
	}
	setState(revert, "undone")
	return nil
}

func (r *reverts) unpause(revert *vs.SnapshotRevert) error {
	glog.V(6).Infof("unpause snapshot revert %v/%v", revert.Namespace, revert.Name)
	sts, err := r.kube.GetSts(revert.Namespace, revert.Spec.StsType.Name)
	if err != nil {
		return e("Unable to get STS for %v revert", err, revert)
	}
	details := &revert.Status.Reverts[len(revert.Status.Reverts)-1]
	err = r.kube.SetStsReplica(sts.Namespace, sts.Name, int(*details.Replicas))
	if err := wait.PollImmediate(10*time.Second, 10*time.Minute, func() (bool, error) { return r.kube.PodsReady(sts) }); err != nil {
		return e("waiting for pods deleted %v", err, sts)
	}
	revert.Spec.Action.Type = "idle"
	setState(revert, "finished")
	return nil
}

func (r *reverts) missingValidation(revert *vs.SnapshotRevert) bool {
	if revert.Spec.Validation == nil || *revert.Spec.Validation == "" {
		return false
	}
	len := len(revert.Status.Reverts)
	if len == 0 || revert.Status.Reverts[len-1].State != "init" {
		return false
	}

	validation, err := r.kube.GetStrategy(revert.Namespace, *revert.Spec.Validation)
	if err != nil {
		return true
	}
	run, err := r.kube.MatchRun(validation)
	if err != nil {
		return true
	}
	if run == nil {
		return true
	}
	if run.Status.ValidationFinished == nil {
		return true
	}
	return false
}

func (r *reverts) runValidation(revert *vs.SnapshotRevert) error {
	validation, err := r.kube.GetStrategy(revert.Namespace, *revert.Spec.Validation)
	if err != nil {
		return e("Unable to get validation %v for revert %v", err, revert.Spec.Validation, revert)
	}
	run, err := r.kube.MatchRun(validation)
	if err != nil {
		return e("Unable to match validation %v for revert %v", err, revert.Spec.Validation, revert)
	}
	if run == nil {
		run, err = r.initRun(validation, revert)
		if err != nil {
			return e("Unable to init run for revert %v", err, revert)
		}
		err := r.kube.CreateValidationRun(run)
		if err != nil {
			return e("Unable to create run for revert %v", err, revert)
		}
	}
	if err := wait.PollImmediate(10*time.Second, 10*time.Minute,
		func() (bool, error) {
			runp, errp := r.kube.GetRun(run.Namespace, run.Name)
			if errp != nil {
				return false, errp
			}
			return runp.Status.ValidationFinished != nil, nil
		}); err != nil {
		return e("waiting for validation run %v", err, run)
	}

	run, err = r.kube.GetRun(run.Namespace, run.Name)
	if err != nil {
		return e("Unable to find validation run %v for revert %v", err, run, revert)
	}
	runCopy := run.DeepCopy()
	runCopy.Spec.Cleanup = true
	if err = r.kube.UpdateValidationRun(runCopy); err != nil {
		return e("Unable to update validation run %v for revert %v", err, run, revert)
	}
	return nil
}

func (r *reverts) initRun(strategy *vs.ValidationStrategy, revert *vs.SnapshotRevert) (*vs.ValidationRun, error) {
	details := &revert.Status.Reverts[len(revert.Status.Reverts)-1]
	snapshots, err := r.getSnapshots(details.OldClaims, revert.Spec.Action.FromTime, revert.Spec.Action.ToTime)
	if err != nil {
		return nil, e("Unable to find required matching snapshots for revert %v", err, revert)
	}
	pvcs := make(map[string]string)
	for pvc, snap := range snapshots {
		pvcs[pvc] = snap.Metadata.Name
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
	return run, nil
}

func (r *reverts) processLatest(revert *vs.SnapshotRevert) error {
	glog.V(4).Infof("processing snapshot revert %v/%v with action type 'latest'", revert.Namespace, revert.Name)
	if r.missingValidation(revert) {
		if err := r.runValidation(revert); err != nil {
			return e("Failed validation for revert %v", err, revert)
		}
	}
	if revert.Spec.StsType != nil {
		details := initDetails(revert)
		switch details.State {
		case "":
			return r.init(revert)
		case "init":
			return r.pause(revert)
		case "paused":
			return r.revertSnapshots(revert)
		case "reverted":
			return r.unpause(revert)
		default:
			return fmt.Errorf("Unknown state '%v' for processing 'latest' action in revert %v/%v", details.State, revert.Namespace, revert.Name)
		}
	}
	return nil
}

func (r *reverts) processUndo(revert *vs.SnapshotRevert) error {
	if revert.Spec.StsType != nil {
		if len(revert.Status.Reverts) == 0 {
			glog.Errorf("no revert to undo %v/%v", revert.Namespace, revert.Name)
			return nil
		}
		details := &revert.Status.Reverts[len(revert.Status.Reverts)-1]
		switch details.State {
		case "finished":
			return r.pause(revert)
		case "paused":
			return r.undoRevertSnapshots(revert)
		case "undone":
			if err := r.unpause(revert); err != nil {
				return err
			}
			revert.Status.Reverts = revert.Status.Reverts[:len(revert.Status.Reverts)-1]
		default:
			return fmt.Errorf("Unknown state '%v' for processing 'undo' action in revert %v/%v", details.State, revert.Namespace, revert.Name)
		}
	}
	return nil
}

func (r *reverts) ProcessSnapshotRevert(revert *vs.SnapshotRevert) error {
	glog.Infof("processing snapshot revert %v/%v", revert.Namespace, revert.Name)
	copy := revert.DeepCopy()
	switch revert.Spec.Action.Type {
	case "latest":
		if err := r.processLatest(copy); err != nil {
			return err
		}
		rl := len(copy.Status.Reverts)
		if copy.Spec.KeepStatus != 0 && rl > copy.Spec.KeepStatus {
			copy.Status.Reverts = copy.Status.Reverts[rl-copy.Spec.KeepStatus:]
		}
	case "undo":
		if err := r.processUndo(copy); err != nil {
			return err
		}
	}
	glog.V(6).Infof("updating shapshot revert %v/%v", revert.Namespace, revert.Name)
	return r.kube.UpdateRevert(copy)
}

func isBetween(pre, target, post *meta.Time) bool {
	if pre != nil {
		if pre.Time.After(target.Time) {
			return false
		}
	}
	if post != nil {
		if post.Time.Before(target.Time) {
			return false
		}
	}
	return true
}

func (r *reverts) getSnapshots(pvcs []*core.PersistentVolumeClaim, fromTime, toTime *meta.Time) (map[string]*snap.VolumeSnapshot, error) {
	snaps, err := r.kube.ListSnapshots()
	if err != nil && len(snaps) < len(pvcs) {
		return nil, e("Failed finding all snapshots for pvcs", err)
	}
	snapMap := make(map[string]*snap.VolumeSnapshot)
	for _, snap := range snaps {
		if !isBetween(fromTime, &snap.Metadata.CreationTimestamp, toTime) {
			continue
		}
		if prev, ok := snapMap[snap.Spec.PersistentVolumeClaimName]; ok {
			if snap.Metadata.CreationTimestamp.Time.After(prev.Metadata.CreationTimestamp.Time) {
				snapMap[snap.Spec.PersistentVolumeClaimName] = snap
			}
		} else {
			snapMap[snap.Spec.PersistentVolumeClaimName] = snap
		}
	}
	m := make(map[string]*snap.VolumeSnapshot)
	for _, pvc := range pvcs {
		if s, ok := snapMap[pvc.Name]; ok {
			m[pvc.Name] = s
		} else {
			return nil, fmt.Errorf("Missing snapshot for pvc %v/%v", pvc.Namespace, pvc.Name)
		}
	}
	return m, nil
}

func setState(revert *vs.SnapshotRevert, state string) {
	details := &revert.Status.Reverts[len(revert.Status.Reverts)-1]
	details.State = state
}

func (r *reverts) availablePV(pv *core.PersistentVolume) error {
	pv.Spec.ClaimRef = nil
	pv.Status.Phase = "Available"
	if err := r.kube.UpdatePV(pv); err != nil {
		return err
	}
	return nil
}

func (r *reverts) fixReclaimPolicy(pv *core.PersistentVolume) error {
	if pv.Spec.PersistentVolumeReclaimPolicy != "Retain" {
		pv.Spec.PersistentVolumeReclaimPolicy = "Retain"
		if err := r.kube.UpdatePV(pv); err != nil {
			return err
		}
	}
	return nil
}
