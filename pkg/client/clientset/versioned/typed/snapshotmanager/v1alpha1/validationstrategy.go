/*
Copyright The Kubernetes Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/cisco-sso/snapshot-manager/pkg/apis/snapshotmanager/v1alpha1"
	scheme "github.com/cisco-sso/snapshot-manager/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ValidationStrategiesGetter has a method to return a ValidationStrategyInterface.
// A group's client should implement this interface.
type ValidationStrategiesGetter interface {
	ValidationStrategies(namespace string) ValidationStrategyInterface
}

// ValidationStrategyInterface has methods to work with ValidationStrategy resources.
type ValidationStrategyInterface interface {
	Create(*v1alpha1.ValidationStrategy) (*v1alpha1.ValidationStrategy, error)
	Update(*v1alpha1.ValidationStrategy) (*v1alpha1.ValidationStrategy, error)
	UpdateStatus(*v1alpha1.ValidationStrategy) (*v1alpha1.ValidationStrategy, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.ValidationStrategy, error)
	List(opts v1.ListOptions) (*v1alpha1.ValidationStrategyList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ValidationStrategy, err error)
	ValidationStrategyExpansion
}

// validationStrategies implements ValidationStrategyInterface
type validationStrategies struct {
	client rest.Interface
	ns     string
}

// newValidationStrategies returns a ValidationStrategies
func newValidationStrategies(c *SnapshotmanagerV1alpha1Client, namespace string) *validationStrategies {
	return &validationStrategies{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the validationStrategy, and returns the corresponding validationStrategy object, and an error if there is any.
func (c *validationStrategies) Get(name string, options v1.GetOptions) (result *v1alpha1.ValidationStrategy, err error) {
	result = &v1alpha1.ValidationStrategy{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("validationstrategies").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ValidationStrategies that match those selectors.
func (c *validationStrategies) List(opts v1.ListOptions) (result *v1alpha1.ValidationStrategyList, err error) {
	result = &v1alpha1.ValidationStrategyList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("validationstrategies").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested validationStrategies.
func (c *validationStrategies) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("validationstrategies").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a validationStrategy and creates it.  Returns the server's representation of the validationStrategy, and an error, if there is any.
func (c *validationStrategies) Create(validationStrategy *v1alpha1.ValidationStrategy) (result *v1alpha1.ValidationStrategy, err error) {
	result = &v1alpha1.ValidationStrategy{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("validationstrategies").
		Body(validationStrategy).
		Do().
		Into(result)
	return
}

// Update takes the representation of a validationStrategy and updates it. Returns the server's representation of the validationStrategy, and an error, if there is any.
func (c *validationStrategies) Update(validationStrategy *v1alpha1.ValidationStrategy) (result *v1alpha1.ValidationStrategy, err error) {
	result = &v1alpha1.ValidationStrategy{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("validationstrategies").
		Name(validationStrategy.Name).
		Body(validationStrategy).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *validationStrategies) UpdateStatus(validationStrategy *v1alpha1.ValidationStrategy) (result *v1alpha1.ValidationStrategy, err error) {
	result = &v1alpha1.ValidationStrategy{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("validationstrategies").
		Name(validationStrategy.Name).
		SubResource("status").
		Body(validationStrategy).
		Do().
		Into(result)
	return
}

// Delete takes name of the validationStrategy and deletes it. Returns an error if one occurs.
func (c *validationStrategies) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("validationstrategies").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *validationStrategies) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("validationstrategies").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched validationStrategy.
func (c *validationStrategies) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ValidationStrategy, err error) {
	result = &v1alpha1.ValidationStrategy{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("validationstrategies").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
