package v1alpha1

import (
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
	MatchLabels   map[string]string `json:"matchLabels"`
	Resources     []string          `json:"resources"` //TODO: check how this is done in "list" object
	InitHook      string            `json:"initHook"`  //TODO: check how this is done in "job" object
	PreValidation string            `json:"preValidation"`
	Validation    string            `json:"validation"`
	KeepFinished  int               `json:"keepFinished"`
	KeepActive    int               `json:"keepActive"`
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
	Snapshots map[string]bool `json:"snapshots"`
}

// ValidationRunStatus
type ValidationRunStatus struct {
	InitStarted           metav1.Time `json:"initStarted"`
	InitFinished          metav1.Time `json:"initFinished"`
	PreValidationStarted  metav1.Time `json:"PreValidationStarted"`
	PreValidationFinished metav1.Time `json:"PreValidationFinished"`
	ValidationStarted     metav1.Time `json:"validationStarted"`
	ValidationFinished    metav1.Time `json:"validationFinished"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ValidationRunList
type ValidationRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ValidationRun `json:"items"`
}
