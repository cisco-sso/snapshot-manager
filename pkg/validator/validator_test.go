package validator

import (
	"fmt"
	vs "github.com/cisco-sso/snapshot-validator/pkg/apis/snapshotvalidator/v1alpha1"
	snap "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"strings"
	"testing"
)

type basicClient struct {
	validationRuns        map[string]*vs.ValidationRun
	newValidationRuns     []*vs.ValidationRun
	updatedValidationRuns []*vs.ValidationRun
	newPVCs               []*core.PersistentVolumeClaim
	newJobs               []*batch.Job
	newSts                []*apps.StatefulSet
	pvcs                  map[string]*core.PersistentVolumeClaim
	jobs                  map[string]*batch.Job
}

func newBasicClient() *basicClient {

	pvc1 := core.PersistentVolumeClaim{}
	pvc1.Name = "data-test-0"
	pvc1.Namespace = "test"
	pvc1.Status.Phase = core.ClaimBound
	pvc2 := core.PersistentVolumeClaim{}
	pvc2.Name = "data-test-1"
	pvc2.Namespace = "test"
	pvc2.Status.Phase = core.ClaimBound

	return &basicClient{
		validationRuns: make(map[string]*vs.ValidationRun),
		pvcs: map[string]*core.PersistentVolumeClaim{
			"data-test-0": &pvc1,
			"data-test-1": &pvc2,
		},
		jobs: make(map[string]*batch.Job),
	}
}
func (c *basicClient) CreateService(*core.Service) error {
	return nil
}
func (c *basicClient) GetSts(string, string) (*apps.StatefulSet, error) {
	sts := apps.StatefulSet{}
	sts.Name = "snapshot-cassandra"
	sts.Namespace = "test"
	var replicas int32
	replicas = 2
	sts.Spec.Replicas = &replicas
	sts.Spec.Selector = &meta.LabelSelector{
		MatchLabels: map[string]string{
			"app": "test",
		},
	}
	return &sts, nil
}
func (c *basicClient) GetService(string, string) (*core.Service, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *basicClient) CreateObjectYAML(string) error {
	return nil
}
func (c *basicClient) GetObjectYAML(ns string, r vs.ResourceName) (string, error) {
	yaml := "kind: " + r.Kind + "\n" +
		"apiVersion: " + r.Version + "\n" +
		"metadata:\n" +
		"  name: " + r.Name + "\n" +
		"  namespace: " + ns + "\n"
	return yaml, nil
}
func (c *basicClient) SetStsReplica(string, string, int) error {
	return fmt.Errorf("not implemented")
}
func (c *basicClient) DeletePVC(pvc *core.PersistentVolumeClaim) error {
	delete(c.pvcs, pvc.Name)
	return nil
}
func (c *basicClient) DeleteValidationRun(*vs.ValidationRun) error {
	return fmt.Errorf("not implemented")
}
func (c *basicClient) GetPV(name string) (*core.PersistentVolume, error) {
	pv := core.PersistentVolume{}
	pv.Name = name
	pv.Spec.Cinder = &core.CinderPersistentVolumeSource{
		VolumeID: "VolumeID",
		FSType:   "FSType",
	}
	return &pv, nil
}
func (c *basicClient) GetSnapshot(ns string, n string) (*snap.VolumeSnapshot, error) {
	s := snap.VolumeSnapshot{}
	s.Metadata.Name = n
	s.Metadata.Namespace = ns
	return &s, nil
}
func (c *basicClient) LabelSnapshot(string, string, string, string) error {
	return nil
}
func (c *basicClient) ListPVCs(selector labels.Selector) ([]*core.PersistentVolumeClaim, error) {
	var pvcs []*core.PersistentVolumeClaim
	for _, v := range c.pvcs {
		pvcs = append(pvcs, v)
	}
	return pvcs, nil
}
func (c *basicClient) ListStrategies() ([]*vs.ValidationStrategy, error) {
	strategy := vs.ValidationStrategy{
		Spec: vs.ValidationStrategySpec{
			AutoTrigger: true,
			StsType: &vs.StatefulSetType{
				Name:  "test",
				Claim: "data",
			},
			AdditionalResources: []vs.ResourceName{},
			Kustomization: vs.Kustomization{
				NamePrefix: "snapshot-",
			},
			Hooks: &vs.Hooks{
				Init:          &batch.JobSpec{},
				PreValidation: &batch.JobSpec{},
				Validation:    &batch.JobSpec{},
			},
		},
	}
	strategy.Name = "basic"
	strategy.Namespace = "test"
	return []*vs.ValidationStrategy{&strategy}, nil
}

func (c *basicClient) ListRuns() ([]*vs.ValidationRun, error) {
	runs := make([]*vs.ValidationRun, 0)
	for _, r := range c.validationRuns {
		runs = append(runs, r.DeepCopy())
	}
	return runs, nil
}
func (c *basicClient) ListPods(labels.Selector) ([]*core.Pod, error) {
	pod1 := core.Pod{}
	pod1.Name = "pod1"
	pod1.Namespace = "test"
	pod1.Labels = map[string]string{"app": "test"}
	pod1.Spec.Volumes = []core.Volume{
		{
			Name: "data",
		},
	}
	pod1.Spec.Volumes[0].PersistentVolumeClaim = &core.PersistentVolumeClaimVolumeSource{
		ClaimName: "data-test-0",
	}
	pod1.Status.Phase = core.PodRunning
	pod2 := core.Pod{}
	pod2.Name = "pod2"
	pod2.Namespace = "test"
	pod2.Labels = map[string]string{"app": "test"}
	pod2.Spec.Volumes = []core.Volume{
		{
			Name: "data",
		},
	}
	pod2.Spec.Volumes[0].PersistentVolumeClaim = &core.PersistentVolumeClaimVolumeSource{
		ClaimName: "data-test-1",
	}
	pod2.Status.Phase = core.PodRunning

	return []*core.Pod{&pod1, &pod2}, nil
}
func (c *basicClient) CreatePVC(pvc *core.PersistentVolumeClaim) error {
	pvc.Status.Phase = core.ClaimBound
	c.newPVCs = append(c.newPVCs, pvc)
	c.pvcs[pvc.Name] = pvc
	return nil
}
func (c *basicClient) CreateJob(j *batch.Job) error {
	c.newJobs = append(c.newJobs, j)
	j.Status.Succeeded = 1
	c.jobs[j.Name] = j
	return nil
}
func (c *basicClient) CreateStatefulSet(s *apps.StatefulSet) error {
	c.newSts = append(c.newSts, s)
	return nil
}
func (c *basicClient) UpdateValidationRun(v *vs.ValidationRun) error {
	c.updatedValidationRuns = append(c.updatedValidationRuns, v)
	c.validationRuns[v.Name] = v
	return nil
}
func (c *basicClient) CreateValidationRun(v *vs.ValidationRun) error {
	c.newValidationRuns = append(c.newValidationRuns, v)
	c.validationRuns[v.Name] = v
	return nil
}
func (c *basicClient) GetPVC(namespace, name string) (*core.PersistentVolumeClaim, error) {
	pvc := c.pvcs[name]
	if pvc == nil {
		return nil, fmt.Errorf("PVC %v not found", name)
	}
	return pvc, nil
}
func (c *basicClient) GetJob(namespace, name string) (*batch.Job, error) {
	job := c.jobs[name]
	if job == nil {
		return nil, fmt.Errorf("job %v not found", name)
	}
	return job, nil
}

func snapshot(name string) *snap.VolumeSnapshot {
	s := snap.VolumeSnapshot{}
	s.Metadata.Name = name + "-snapshot"
	s.Metadata.Namespace = "test"
	s.Spec.PersistentVolumeClaimName = name
	return &s
}

func TestBasicSnapshotPVC1(t *testing.T) {
	c := newBasicClient()
	v := NewValidator(c)
	s := snapshot("data-test-0")
	err := v.ProcessSnapshot(s)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.newValidationRuns) != 1 {
		t.Fatal("Expected 1 new validation run but got", len(c.newValidationRuns))
	}
	if len(c.updatedValidationRuns) != 0 {
		t.Fatal("Expected 0 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run := c.newValidationRuns[0]
	if len(run.Spec.ClaimsToSnapshots) != 2 {
		t.Fatal("Expected 2 PVCs in snapshot run but got", len(run.Spec.ClaimsToSnapshots))
	}
	if run.Spec.ClaimsToSnapshots[s.Spec.PersistentVolumeClaimName] != s.Metadata.Name {
		t.Fatal("Expected run snapshot", s.Spec.PersistentVolumeClaimName,
			"to point to snapshot", s.Metadata.Name, "but is pointing to", run.Spec.ClaimsToSnapshots[s.Spec.PersistentVolumeClaimName])
	}
}

func TestBasicSnapshotPVC1and2(t *testing.T) {
	c := newBasicClient()
	v := NewValidator(c)
	s1 := snapshot("data-test-0")
	err := v.ProcessSnapshot(s1)
	if err != nil {
		t.Error("Expected no error but got", err)
	}
	s2 := snapshot("data-test-1")
	err = v.ProcessSnapshot(s2)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.newValidationRuns) != 1 {
		t.Fatal("Expected 1 new validation run but got", len(c.newValidationRuns))
	}
	if len(c.updatedValidationRuns) != 1 {
		t.Fatal("Expected 1 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run := c.updatedValidationRuns[0]
	if len(run.Spec.ClaimsToSnapshots) != 2 {
		t.Fatal("Expected 2 PVCs in snapshot run but got", len(run.Spec.ClaimsToSnapshots))
	}
	if run.Spec.ClaimsToSnapshots[s1.Spec.PersistentVolumeClaimName] != s1.Metadata.Name {
		t.Fatal("Expected run snapshot", s1.Spec.PersistentVolumeClaimName,
			"to point to snapshot", s1.Metadata.Name, "but is pointing to", run.Spec.ClaimsToSnapshots[s1.Spec.PersistentVolumeClaimName])
	}
	if run.Spec.ClaimsToSnapshots[s2.Spec.PersistentVolumeClaimName] != s2.Metadata.Name {
		t.Fatal("Expected run snapshot", s2.Spec.PersistentVolumeClaimName,
			"to point to snapshot", s2.Metadata.Name, "but is pointing to", run.Spec.ClaimsToSnapshots[s2.Spec.PersistentVolumeClaimName])
	}
}

func TestBasicFlow(t *testing.T) {
	c := newBasicClient()
	v := NewValidator(c)
	s1 := snapshot("data-test-0")
	err := v.ProcessSnapshot(s1)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	err = v.ProcessValidationRun(c.newValidationRuns[0])
	if err != nil && !strings.HasSuffix(err.Error(), "is missing snapshot") {
		t.Fatal("Expected error mismatch", err)
	}
	s2 := snapshot("data-test-1")
	err = v.ProcessSnapshot(s2)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	err = v.ProcessValidationRun(c.updatedValidationRuns[0])
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.updatedValidationRuns) != 2 {
		t.Fatal("Expected 2 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run := c.updatedValidationRuns[1]
	if run.Status.KustStarted == nil || run.Status.KustFinished != nil {
		t.Fatal("Expected kust to have started")
	}
	err = v.ProcessValidationRun(run)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.updatedValidationRuns) != 3 {
		t.Fatal("Expected 3 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run = c.updatedValidationRuns[2]
	if run.Status.KustFinished == nil || run.Status.InitStarted != nil {
		t.Fatal("Expected kust to have finished")
	}
	if len(run.Spec.Objects.Claims) != 2 {
		t.Fatal("Expected 2 kustomized claims but got", len(run.Spec.Objects.Claims))
	}
	if len(run.Spec.Objects.Kustomized) != 1 {
		t.Fatal("Expected 1 kustomized object", len(run.Spec.Objects.Kustomized))
	}

	err = v.ProcessValidationRun(run)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.updatedValidationRuns) != 4 {
		t.Fatal("Expected 4 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run = c.updatedValidationRuns[3]
	if run.Status.InitStarted == nil || run.Status.InitFinished != nil {
		t.Fatal("Expected init to have started")
	}

	err = v.ProcessValidationRun(run)
	if err != nil {
		t.Fatal("Expected no error but got", err)
	}
	if len(c.updatedValidationRuns) != 5 {
		t.Fatal("Expected 5 updated validation runs but got", len(c.updatedValidationRuns))
	}
	run = c.updatedValidationRuns[4]
	if run.Status.InitFinished == nil || run.Status.PreValidationStarted != nil {
		t.Fatal("Expected init to have finished")
	}
	if len(c.newPVCs) != 2 {
		t.Fatal("Expected 2 new PVCs but got", len(c.newPVCs))
	}
	if len(c.newJobs) != 1 {
		t.Fatal("Expected 1 init job but got", len(c.newJobs))
	}
	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[5]
	if run.Status.PreValidationStarted == nil || run.Status.PreValidationFinished != nil {
		t.Fatal("Expected pre-validation to have started")
	}
	if len(c.newJobs) != 1 {
		t.Fatal("Expected 1 init job but got", len(c.newJobs))
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[6]
	if run.Status.PreValidationFinished == nil || run.Status.ValidationStarted != nil {
		t.Fatal("Expected pre-validation to have finished")
	}
	if len(c.newJobs) != 2 {
		t.Fatal("Expected 2 job but got", len(c.newJobs))
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[7]
	if run.Status.ValidationStarted == nil || run.Status.ValidationFinished != nil {
		t.Fatal("Expected validation to have started")
	}
	if len(c.newJobs) != 2 {
		t.Fatal("Expected 2 job but got", len(c.newJobs))
	}

	err = v.ProcessValidationRun(run)
	run = c.updatedValidationRuns[8]
	if run.Status.ValidationFinished == nil {
		t.Fatal("Expected validation to have finished")
	}
	if len(c.newJobs) != 3 {
		t.Fatal("Expected 3 job but got", len(c.newJobs))
	}
}
