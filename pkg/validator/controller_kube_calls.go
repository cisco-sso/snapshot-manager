package validator

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotvalidator/v1alpha1"
	"github.com/golang/glog"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	//"k8s.io/apimachinery/pkg/runtime"
	"bytes"
	"k8s.io/apimachinery/pkg/types"
	"os/exec"
	"strings"
)

type KubeCalls interface {
	pods
	services
	persistentVolumeClaims
	persistentVolumes
	jobs
	validationStrategies
	validationRuns
	snapshots
	statefulSets
	CreateObjectYAML(string) error
	GetObjectYAML(string, vs.ResourceName) (string, error)
}

type pods interface {
	ListPods(labels.Selector) ([]*core.Pod, error)
}

type services interface {
	CreateService(*core.Service) error
	GetService(string, string) (*core.Service, error)
}

type persistentVolumeClaims interface {
	GetPVC(string, string) (*core.PersistentVolumeClaim, error)
	CreatePVC(*core.PersistentVolumeClaim) error
	DeletePVC(*core.PersistentVolumeClaim) error
	ListPVCs(selector labels.Selector) ([]*core.PersistentVolumeClaim, error)
}

type persistentVolumes interface {
	GetPV(string) (*core.PersistentVolume, error)
}

type jobs interface {
	CreateJob(*batch.Job) error
	GetJob(string, string) (*batch.Job, error)
}

type validationStrategies interface {
	ListStrategies() ([]*vs.ValidationStrategy, error)
}

type validationRuns interface {
	ListRuns() ([]*vs.ValidationRun, error)
	CreateValidationRun(*vs.ValidationRun) error
	UpdateValidationRun(*vs.ValidationRun) error
	DeleteValidationRun(*vs.ValidationRun) error
}

type snapshots interface {
	GetSnapshot(string, string) (*snap.VolumeSnapshot, error)
	LabelSnapshot(string, string, string, string) error
}

type statefulSets interface {
	CreateStatefulSet(*apps.StatefulSet) error
	GetSts(string, string) (*apps.StatefulSet, error)
	SetStsReplica(string, string, int) error
}

func (c *controller) GetObjectYAML(namespace string, r vs.ResourceName) (string, error) {
	//TODO: do this intelligently without calling kubectl binary
	//maybe k8s.io/client-go/rest/config.go
	//or look at how kubectl does this
	cmd := exec.Command("kubectl", "get", "-o", "yaml", "-n", namespace, r.Kind, r.Name)
	stdout := bytes.NewBufferString("")
	cmd.Stdout = stdout
	stderr := bytes.NewBufferString("")
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		glog.Errorf("Unable to get object from YAML %v: %v: %v", stderr.String(), stdout.String(), r)
		return "", e("unable to get object from YAML %v", err, stderr.String())
	}
	return stdout.String(), nil
}

func (c *controller) CreateObjectYAML(str string) error {
	//TODO: do this intelligently without calling kubectl binary
	//maybe k8s.io/client-go/rest/config.go
	//or look at how kubectl does this
	cmd := exec.Command("kubectl", "create", "-f", "-")
	cmd.Stdin = strings.NewReader(str)
	stdout := bytes.NewBufferString("")
	cmd.Stdout = stdout
	stderr := bytes.NewBufferString("")
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		glog.Error("Unable to create object from YAML ", stderr.String(), ", ", stdout.String(), ", ", str)
		return e("unable to create object from YAML %v", err, stderr.String())
	}
	return nil
}

func (c *controller) UpdateValidationRun(run *vs.ValidationRun) error {
	_, err := c.clients.SvClientset.SnapshotvalidatorV1alpha1().
		ValidationRuns(run.Namespace).
		Update(run)
	return err
}

func (c *controller) CreateValidationRun(run *vs.ValidationRun) error {
	_, err := c.clients.SvClientset.SnapshotvalidatorV1alpha1().
		ValidationRuns(run.Namespace).
		Create(run)
	return err
}

func (c *controller) DeleteValidationRun(run *vs.ValidationRun) error {
	return c.clients.SvClientset.SnapshotvalidatorV1alpha1().
		ValidationRuns(run.Namespace).
		Delete(run.Name, &meta.DeleteOptions{})
}

func (c *controller) CreatePVC(pvc *core.PersistentVolumeClaim) error {
	_, err := c.clients.KubeClientset.CoreV1().
		PersistentVolumeClaims(pvc.Namespace).
		Create(pvc)
	return err
}

func (c *controller) DeletePVC(pvc *core.PersistentVolumeClaim) error {
	return c.clients.KubeClientset.CoreV1().
		PersistentVolumeClaims(pvc.Namespace).
		Delete(pvc.Name, &meta.DeleteOptions{})
}

func (c *controller) CreateStatefulSet(sts *apps.StatefulSet) error {
	_, err := c.clients.KubeClientset.AppsV1().
		StatefulSets(sts.Namespace).
		Create(sts)
	return err
}

func (c *controller) CreateJob(job *batch.Job) error {
	_, err := c.clients.KubeClientset.BatchV1().
		Jobs(job.Namespace).
		Create(job)
	return err
}

func (c *controller) CreateService(svc *core.Service) error {
	_, err := c.clients.KubeClientset.CoreV1().
		Services(svc.Namespace).
		Create(svc)
	return err
}

func (c *controller) LabelSnapshot(namespace, name, label, key string) error {
	//TODO: patch
	//patch := fmt.Sprintf(`[{"op":"replace","path":"/metadata/labels/%v","value":%v}]`, label, key)
	var result snap.VolumeSnapshot
	s, err := c.GetSnapshot(namespace, name)
	if err != nil {
		return fmt.Errorf("Failed getting snapshot %v/%v for relabel: %v=%v", namespace, name, label, key)
	}
	copy := s.DeepCopy()
	copy.Metadata.Labels[label] = key
	return c.clients.SnapshotClient.Put().
		Resource(snap.VolumeSnapshotResourcePlural).
		Namespace(namespace).
		Name(name).
		Body(copy).
		Do().Into(&result)
}

func (c *controller) ListPods(selector labels.Selector) ([]*core.Pod, error) {
	return c.podLister.List(selector)
}

func (c *controller) ListPVCs(selector labels.Selector) ([]*core.PersistentVolumeClaim, error) {
	return c.pvcLister.List(selector)
}

func (c *controller) ListStrategies() ([]*vs.ValidationStrategy, error) {
	if err := c.vsInformer.GetStore().Resync(); err != nil {
		return nil, e("Failed to resync vsInformer store", err)
	}
	return c.vsLister.List(labels.Everything())
}

func (c *controller) ListRuns() ([]*vs.ValidationRun, error) {
	if err := c.vrInformer.GetStore().Resync(); err != nil {
		return nil, e("Failed to resync vrInformer store", err)
	}
	return c.vrLister.List(labels.Everything())
}

func (c *controller) GetPVC(namespace, name string) (*core.PersistentVolumeClaim, error) {
	return c.pvcLister.PersistentVolumeClaims(namespace).Get(name)
}

func (c *controller) GetPV(name string) (*core.PersistentVolume, error) {
	return c.pvLister.Get(name)
}

func (c *controller) GetJob(namespace, name string) (*batch.Job, error) {
	if err := c.jobInformer.GetStore().Resync(); err != nil {
		return nil, e("Failed to resync jobsInformer store", err)
	}
	return c.jobLister.Jobs(namespace).Get(name)
}

func (c *controller) GetSts(namespace, name string) (*apps.StatefulSet, error) {
	return c.stsLister.StatefulSets(namespace).Get(name)
}

func (c *controller) GetSnapshot(namespace, name string) (*snap.VolumeSnapshot, error) {
	key := namespace + "/" + name
	obj, ok, err := c.snapshotStore.GetByKey(key)
	if !ok {
		return nil, fmt.Errorf("Snapshot %v not found", key)
	}
	if err != nil {
		return nil, fmt.Errorf("Finding Snapshot %v failed: %v", key, err)
	}
	snapshot, ok := obj.(*snap.VolumeSnapshot)
	if !ok {
		return nil, fmt.Errorf("Finding Snapshot %v failed typecast", key)
	}
	return snapshot, nil
}

func (c *controller) SetStsReplica(namespace, name string, replica int) error {
	patch := fmt.Sprintf(`[{"op":"replace","path":"/spec/replicas","value":%d}]`, replica)
	_, err := c.clients.KubeClientset.AppsV1().StatefulSets(namespace).
		Patch(name, types.JSONPatchType, []byte(patch))
	return err
}

func (c *controller) GetService(namespace, name string) (*core.Service, error) {
	return c.serviceLister.Services(namespace).Get(name)
}
