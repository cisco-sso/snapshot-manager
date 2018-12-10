package v1alpha1

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const (
	ValidationStrategyResource       = "ValidationStrategy"
	ValidationStrategyResourcePlural = "ValidationStrategies"
	ValidationRunResource            = "ValidationRun"
	ValidationRunResourcePlural      = "ValidationRuns"
	SnapshotRevertResource           = "SnapshotRevert"
	SnapshotRevertResourcePlural     = "SnapshotReverts"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SnapshotRevert
type SnapshotRevert struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SnapshotRevertSpec   `json:"spec"`
	Status SnapshotRevertStatus `json:"status"`
}

// SnapshotRevertSpec
type SnapshotRevertSpec struct {
	StsType *StatefulSetType `json:"statefulSet,omitempty"`

	Validation *string              `json:"validation,omitempty"`
	Action     SnapshotRevertAction `json:"action"`
}

// SnapshotRevertAction
type SnapshotRevertAction struct {
	Type     string       `json:"type"`
	FromTime *metav1.Time `json:"fromTime,omitempty"`
	ToTime   *metav1.Time `json:"toTime,omitempty"`
}

// SnapshotRevertStatus
type SnapshotRevertStatus struct {
	Reverts []SnapshotRevertDetails `json:"reverts,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationStrategyList
type SnapshotRevertList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []SnapshotRevert `json:"items"`
}

// SnapshotRevertDetails
type SnapshotRevertDetails struct {
	TargetTime  *metav1.Time      `json:"targetTime,omitempty"`
	PreVolumes  map[string]string `json:"preVolumes"`
	PrePVs      map[string]string `json:"prePVs"`
	PostVolumes map[string]string `json:"postVolumes"`
	PostPVs     map[string]string `json:"postPVs"`
}

func (r *SnapshotRevert) AttachSnapshot(pvc *corev1.PersistentVolumeClaim, snap string) *corev1.PersistentVolumeClaim {
	newClaim := &corev1.PersistentVolumeClaim{}
	if r.Spec.StsType != nil {
		newClaim.Spec.StorageClassName = r.Spec.StsType.SnapshotClaimStorageClass
	}
	newClaim.Name = pvc.Name
	newClaim.Labels = pvc.Labels
	newClaim.Annotations = map[string]string{"snapshot.alpha.kubernetes.io/snapshot": snap}
	newClaim.Namespace = pvc.Namespace
	newClaim.Spec.AccessModes = pvc.Spec.AccessModes
	newClaim.Spec.Resources = pvc.Spec.Resources
	return newClaim
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationStrategy
type ValidationStrategy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ValidationStrategySpec   `json:"spec"`
	Status ValidationStrategyStatus `json:"status"`
}

func (vs ValidationStrategy) GetKustResources() []ResourceName {
	var allResources []ResourceName
	if vs.Spec.StsType != nil {
		allResources = append(allResources, ResourceName{Kind: "StatefulSet", Name: vs.Spec.StsType.Name})
	}
	allResources = append(allResources, vs.Spec.AdditionalResources...)
	return allResources
}

func (vs ValidationStrategy) KustomizeClaims(claims map[string]*corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	var kustClaims []corev1.PersistentVolumeClaim
	for snap, claim := range claims {
		kustClaim := corev1.PersistentVolumeClaim{}
		if vs.Spec.StsType != nil {
			nameSplit := strings.Split(claim.Name, "-")
			id := nameSplit[len(nameSplit)-1]
			kustClaim.Name = strings.Join([]string{vs.Spec.StsType.Claim, vs.Spec.Kustomization.NamePrefix + vs.Spec.StsType.Name, id}, "-")
			kustClaim.Spec.StorageClassName = vs.Spec.StsType.SnapshotClaimStorageClass
		}
		kustClaim.Labels = make(map[string]string)
		for k, v := range vs.Spec.Kustomization.CommonLabels {
			kustClaim.Labels[k] = v
		}
		kustClaim.Annotations = map[string]string{"snapshot.alpha.kubernetes.io/snapshot": snap}
		for k, v := range vs.Spec.Kustomization.CommonAnnotations {
			kustClaim.Annotations[k] = v
		}
		kustClaim.Namespace = claim.Namespace
		kustClaim.Spec.AccessModes = claim.Spec.AccessModes
		kustClaim.Spec.Resources = claim.Spec.Resources
		kustClaims = append(kustClaims, kustClaim)
	}
	return kustClaims
}

// ValidationStrategySpec
type ValidationStrategySpec struct {
	StsType *StatefulSetType `json:"statefulSet,omitempty"`

	AdditionalResources []ResourceName `json:"additionalResources"`
	Kustomization       Kustomization  `json:"kustomization"`
	Hooks               *Hooks         `json:"hooks,omitempty"`
	AutoTrigger         bool           `json:"autoTrigger"`
}

// Hooks
type Hooks struct {
	Init          *batchv1.JobSpec `json:"init,omitempty"`
	PreValidation *batchv1.JobSpec `json:"preValidation,omitempty"`
	Validation    *batchv1.JobSpec `json:"validation,omitempty"`
}

// StetfulSetStrategy
type StatefulSetType struct {
	Name                      string  `json:"name"`
	Claim                     string  `json:"claim"`
	SnapshotClaimStorageClass *string `json:"snapshotClaimStorageClass,omitempty"`
}

// Kustomization
type Kustomization struct {
	NamePrefix        string            `json:"namePrefix,omitempty"`
	CommonLabels      map[string]string `json:"commonLabels,omitempty"`
	CommonAnnotations map[string]string `json:"commonAnnotations,omitempty"`
	Patches           map[string]string `json:"patches",omitempty`
}

// Resource
type ResourceName struct {
	Group   string `json:"group,omitempty"`
	Version string `json:"version,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Name    string `json:"name"`
}

func (r *ResourceName) Id() string {
	var sb []string
	if r.Group != "" {
		sb = append(sb, r.Group)
	}
	if r.Version != "" {
		sb = append(sb, r.Version)
	}
	if r.Kind != "" {
		sb = append(sb, r.Kind)
	}
	if r.Name != "" {
		sb = append(sb, r.Name)
	}
	return strings.Join(sb, "/")
}

// ValidationStrategyStatus
type ValidationStrategyStatus struct {
	finishedRuns []string `json:"finishedRuns"`
	activeRuns   []string `json:"activeRuns"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationStrategyList
type ValidationStrategyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ValidationStrategy `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationRun
type ValidationRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ValidationRunSpec   `json:"spec"`
	Status            ValidationRunStatus `json:"status"`
}

// ValidationRunSpec
type ValidationRunSpec struct {
	Suffix            string               `json:"suffix"`
	ClaimsToSnapshots map[string]string    `json:"claimsToSnapshots"`
	Cleanup           bool                 `json:"cleanup"`
	Objects           ValidationRunObjects `json:"objects"`
}

type ValidationRunObjects struct {
	Claims     []corev1.PersistentVolumeClaim `json:"claims"`
	Kustomized []string                       `json:"rest"`
}

// ValidationRunStatus
type ValidationRunStatus struct {
	KustStarted           *metav1.Time `json:"kustStarted,omitempty"`
	KustFinished          *metav1.Time `json:"kustFinished,omitempty"`
	InitStarted           *metav1.Time `json:"initStarted,omitempty"`
	InitFinished          *metav1.Time `json:"initFinished,omitempty"`
	PreValidationStarted  *metav1.Time `json:"preValidationStarted,omitempty"`
	PreValidationFinished *metav1.Time `json:"preValidationFinished,omitempty"`
	ValidationStarted     *metav1.Time `json:"validationStarted,omitempty"`
	ValidationFinished    *metav1.Time `json:"validationFinished,omitempty"`
	CleanupStarted        *metav1.Time `json:"cleanupStarted,omitempty"`
	CleanupFinished       *metav1.Time `json:"cleanupFinished,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationRunList
type ValidationRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ValidationRun `json:"items"`
}
