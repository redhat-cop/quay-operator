/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"errors"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"

	quaycontext "github.com/quay/quay-operator/pkg/context"
)

type QuayVersion string

const (
	QuayVersionCurrent  QuayVersion = "vader"
	QuayVersionPrevious QuayVersion = ""
)

// CanUpgrade returns true if the given version can be upgraded to this Operator's synchronized Quay version.
func CanUpgrade(fromVersion QuayVersion) bool {
	return fromVersion == QuayVersionCurrent || fromVersion == QuayVersionPrevious
}

var allComponents = []string{
	"postgres",
	"clair",
	"redis",
	"horizontalpodautoscaler",
	"objectstorage",
	"route",
	"mirror",
}

var requiredComponents = []string{
	"postgres",
	"objectstorage",
}

// QuayRegistrySpec defines the desired state of QuayRegistry.
type QuayRegistrySpec struct {
	// ConfigBundleSecret is the name of the Kubernetes `Secret` in the same namespace which contains the base Quay config and extra certs.
	ConfigBundleSecret string `json:"configBundleSecret,omitempty"`
	// Components declare how the Operator should handle backing Quay services.
	Components []Component `json:"components,omitempty"`
}

// Component describes how the Operator should handle a backing Quay service.
type Component struct {
	// Kind is the unique name of this type of component.
	Kind string `json:"kind"`
	// Managed indicates whether or not the Operator is responsible for the lifecycle of this component.
	// Default is true.
	Managed bool `json:"managed"`
}

type ConditionType string

const (
	ConditionTypeAvailable      ConditionType = "Available"
	ConditionTypeRolloutBlocked ConditionType = "RolloutBlocked"
	// TODO: Add more useful conditions.
)

type ConditionReason string

const (
	// ConditionTypeAvailable
	ConditionReasonHealthChecksPassing  ConditionReason = "HealthChecksPassing"
	ConditionReasonMigrationsInProgress ConditionReason = "MigrationsInProgress"
	// ConditionTypeRolloutBlocked
	ConditionReasonComponentsCreationSuccess             ConditionReason = "ComponentsCreationSuccess"
	ConditionReasonUpgradeUnsupported                    ConditionReason = "UpgradeUnsupported"
	ConditionReasonComponentCreationFailed               ConditionReason = "ComponentCreationFailed"
	ConditionReasonRouteComponentDependencyError         ConditionReason = "RouteComponentDependencyError"
	ConditionReasonObjectStorageComponentDependencyError ConditionReason = "ObjectStorageComponentDependencyError"
	ConditionReasonConfigInvalid                         ConditionReason = "ConfigInvalid"
)

// Condition is a single condition of a QuayRegistry.
// Conditions should follow the "abnormal-true" principle in order to only bring the attention of users to "broken" states.
// Example: a condition of `type: "Ready", status: "True"`` is less useful and should be omitted whereas `type: "NotReady", status: "True"`
// is more useful when trying to monitor when something is wrong.
type Condition struct {
	Type               ConditionType          `json:"type,omitempty"`
	Status             metav1.ConditionStatus `json:"status,omitempty"`
	Reason             ConditionReason        `json:"reason,omitempty"`
	Message            string                 `json:"message,omitempty"`
	LastUpdateTime     metav1.Time            `json:"lastUpdateTime,omitempty"`
	LastTransitionTime metav1.Time            `json:"lastTransitionTime,omitempty"`
}

// QuayRegistryStatus defines the observed state of QuayRegistry.
type QuayRegistryStatus struct {
	// CurrentVersion is the actual version of Quay that is actively deployed.
	CurrentVersion QuayVersion `json:"currentVersion,omitempty"`
	// RegistryEndpoint is the external access point for the Quay registry.
	RegistryEndpoint string `json:"registryEndpoint,omitempty"`
	// LastUpdate is the timestamp when the Operator last processed this instance.
	LastUpdate string `json:"lastUpdated,omitempty"`
	// ConfigEditorEndpoint is the external access point for a web-based reconfiguration interface
	// for the Quay registry instance.
	ConfigEditorEndpoint string `json:"configEditorEndpoint,omitempty"`
	// ConfigEditorCredentialsSecret is the Kubernetes `Secret` containing the config editor password.
	ConfigEditorCredentialsSecret string `json:"configEditorCredentialsSecret,omitempty"`
	// Conditions represent the conditions that a QuayRegistry can have.
	Conditions []Condition `json:"conditions,omitempty"`
}

// GetCondition retrieves the condition with the matching type from the given list.
func GetCondition(conditions []Condition, conditionType ConditionType) *Condition {
	for _, c := range conditions {
		if c.Type == conditionType {
			return &c
		}
	}

	return nil
}

// SetCondition adds or updates a given condition.
// TODO: Use https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/apimachinery/pkg/api/meta/conditions.go when we can.
func SetCondition(existing []Condition, newCondition Condition) []Condition {
	if existing == nil {
		existing = []Condition{}
	}

	for i, existingCondition := range existing {
		if existingCondition.Type == newCondition.Type {
			existing[i] = newCondition
			return existing
		}
	}

	return append(existing, newCondition)
}

// RemoveCondition removes any conditions with the matching type.
func RemoveCondition(conditions []Condition, conditionType ConditionType) []Condition {
	if conditions == nil {
		return []Condition{}
	}

	filtered := []Condition{}
	for _, existingCondition := range conditions {
		if existingCondition.Type != conditionType {
			filtered = append(filtered, existingCondition)
		}
	}

	return filtered
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// QuayRegistry is the Schema for the quayregistries API.
type QuayRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   QuayRegistrySpec   `json:"spec,omitempty"`
	Status QuayRegistryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// QuayRegistryList contains a list of QuayRegistry.
type QuayRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []QuayRegistry `json:"items"`
}

func EnsureComponents(components []Component) []Component {
	return append(components, components[0])[1 : len(components)+1]
}

// ComponentsMatch returns true if both set of components are equivalent, and false otherwise.
func ComponentsMatch(firstComponents, secondComponents []Component) bool {
	if len(firstComponents) != len(secondComponents) {
		return false
	}

	for _, compA := range firstComponents {
		equal := false
		for _, compB := range secondComponents {
			if compA.Kind == compB.Kind && compA.Managed == compB.Managed {
				equal = true
				break
			}
		}
		if !equal {
			return false
		}
	}
	return true
}

// ComponentIsManaged returns whether the given component is managed or not.
func ComponentIsManaged(components []Component, name string) bool {
	for _, c := range components {
		if c.Kind == name {
			return c.Managed
		}
	}
	return false
}

// RequiredComponent returns whether the given component is required for Quay or not.
func RequiredComponent(component string) bool {
	for _, c := range requiredComponents {
		if c == component {
			return true
		}
	}
	return false
}

// EnsureDefaultComponents adds any `Components` which are missing from `Spec.Components`
// and returns a new `QuayRegistry` copy.
func EnsureDefaultComponents(ctx *quaycontext.QuayRegistryContext, quay *QuayRegistry) (*QuayRegistry, error) {
	updatedQuay := quay.DeepCopy()
	if updatedQuay.Spec.Components == nil {
		updatedQuay.Spec.Components = []Component{}
	}

	if ComponentIsManaged(updatedQuay.Spec.Components, "route") && !ctx.SupportsRoutes {
		return nil, errors.New("cannot use `route` component when `Route` API not available")
	}

	if ComponentIsManaged(updatedQuay.Spec.Components, "objectstorage") && !ctx.SupportsObjectStorage {
		return nil, errors.New("cannot use `objectstorage` component when `ObjectBucketClaims` API not available")
	}

	for _, component := range allComponents {
		found := false
		for _, definedComponent := range quay.Spec.Components {
			if component == definedComponent.Kind {
				found = true
				break
			}
		}

		if !found {
			if component == "route" && !ctx.SupportsRoutes {
				continue
			}
			if component == "objectstorage" && !ctx.SupportsObjectStorage {
				continue
			}

			updatedQuay.Spec.Components = append(updatedQuay.Spec.Components, Component{Kind: component, Managed: true})
		}
	}

	return updatedQuay, nil
}

// EnsureRegistryEndpoint sets the `status.registryEndpoint` field and returns `ok` if it was unchanged.
func EnsureRegistryEndpoint(ctx *quaycontext.QuayRegistryContext, quay *QuayRegistry, config map[string]interface{}) (*QuayRegistry, bool) {
	updatedQuay := quay.DeepCopy()

	if config == nil {
		config = map[string]interface{}{}
	}

	if serverHostname, ok := config["SERVER_HOSTNAME"]; ok {
		updatedQuay.Status.RegistryEndpoint = "https://" + serverHostname.(string)
	} else if ctx.SupportsRoutes {
		updatedQuay.Status.RegistryEndpoint = "https://" + strings.Join([]string{
			strings.Join([]string{quay.GetName(), "quay", quay.GetNamespace()}, "-"),
			ctx.ClusterHostname},
			".")
	}
	// TODO: Retrieve load balancer IP from `Service`

	return updatedQuay, quay.Status.RegistryEndpoint == updatedQuay.Status.RegistryEndpoint
}

// EnsureConfigEditorEndpoint sets the `status.configEditorEndpoint` field and returns `ok` if it was unchanged.
func EnsureConfigEditorEndpoint(ctx *quaycontext.QuayRegistryContext, quay *QuayRegistry) (*QuayRegistry, bool) {
	updatedQuay := quay.DeepCopy()

	if ctx.SupportsRoutes {
		updatedQuay.Status.ConfigEditorEndpoint = "https://" + strings.Join([]string{
			strings.Join([]string{quay.GetName(), "quay-config-editor", quay.GetNamespace()}, "-"),
			ctx.ClusterHostname},
			".")
	}
	// TODO: Retrieve load balancer IP from `Service`

	return updatedQuay, quay.Status.ConfigEditorEndpoint == updatedQuay.Status.ConfigEditorEndpoint
}

// EnsureOwnerReference adds an `ownerReference` to the given object if it does not already have one.
func EnsureOwnerReference(quay *QuayRegistry, obj runtime.Object) (runtime.Object, error) {
	objectMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}

	hasOwnerRef := false
	for _, ownerRef := range objectMeta.GetOwnerReferences() {
		if ownerRef.Name == quay.GetName() &&
			ownerRef.Kind == "QuayRegistry" &&
			ownerRef.APIVersion == GroupVersion.String() &&
			ownerRef.UID == quay.UID {
			hasOwnerRef = true
		}
	}

	if !hasOwnerRef {
		objectMeta.SetOwnerReferences(append(objectMeta.GetOwnerReferences(), metav1.OwnerReference{
			APIVersion: GroupVersion.String(),
			Kind:       "QuayRegistry",
			Name:       quay.GetName(),
			UID:        quay.GetUID(),
		}))
	}

	return obj, nil
}

const ManagedKeysName = "quay-registry-managed-secret-keys"

// ManagedKeysSecretNameFor returns the name of the `Secret` in which generated secret keys are stored.
func ManagedKeysSecretNameFor(quay *QuayRegistry) string {
	return strings.Join([]string{quay.GetName(), ManagedKeysName}, "-")
}

func IsManagedKeysSecretFor(quay *QuayRegistry, secret *corev1.Secret) bool {
	return strings.Contains(secret.GetName(), quay.GetName()+"-"+ManagedKeysName)
}

func init() {
	SchemeBuilder.Register(&QuayRegistry{}, &QuayRegistryList{})
}
