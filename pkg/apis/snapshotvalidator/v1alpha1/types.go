package v1alpha1

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ValidationStrategyResource       = "ValidationStrategy"
	ValidationStrategyResourcePlural = "ValidationStrategies"
	ValidationRunResource            = "ValidationRun"
	ValidationRunResourcePlural      = "ValidationRuns"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationStrategy
type ValidationStrategy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ValidationStrategySpec   `json:"spec"`
	Status ValidationStrategyStatus `json:"status"`
}

// ValidationStrategySpec
type ValidationStrategySpec struct {
	Selector metav1.LabelSelector `json:"selector"`
	//TODO: remove from here and keep in run only
	Claims        map[string]corev1.PersistentVolumeClaim `json:"claims"`
	StatefulSet   *appsv1.StatefulSet                     `json:"statefulSet,omitempty"`
	Service       *corev1.Service                         `json:"service,omitempty"`
	Init          *batchv1.Job                            `json:"init,omitempty"`
	PreValidation *batchv1.Job                            `json:"preValidation,omitempty"`
	Validation    *batchv1.Job                            `json:"validation,omitempty"`
	KeepFinished  int                                     `json:"keepFinished"`
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
	Snapshots map[string]string                       `json:"snapshots"`
	Claims    map[string]corev1.PersistentVolumeClaim `json:"claims"`
}

// ValidationRunStatus
type ValidationRunStatus struct {
	InitStarted           *metav1.Time `json:"initStarted,omitempty"`
	InitFinished          *metav1.Time `json:"initFinished,omitempty"`
	PreValidationStarted  *metav1.Time `json:"PreValidationStarted,omitempty"`
	PreValidationFinished *metav1.Time `json:"PreValidationFinished,omitempty"`
	ValidationStarted     *metav1.Time `json:"validationStarted,omitempty"`
	ValidationFinished    *metav1.Time `json:"validationFinished,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationRunList
type ValidationRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ValidationRun `json:"items"`
}
