/*
Copyright © 2019 Hua Zhihao <ihuazhihao@gmail.com>

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
package cmd

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/remotecommand"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/cmd/attach"
	"k8s.io/kubectl/pkg/cmd/delete"
	"k8s.io/kubectl/pkg/cmd/exec"
	"k8s.io/kubectl/pkg/cmd/logs"
	"k8s.io/kubectl/pkg/cmd/run"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/generate"
	generateversioned "k8s.io/kubectl/pkg/generate/versioned"
	"k8s.io/kubectl/pkg/polymorphichelpers"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
	"k8s.io/kubectl/pkg/util/interrupt"
	uexec "k8s.io/utils/exec"
)

var (
	spyExample = `
		# Spy the first container nginx from pod mypod
		kubectl spy mypod

		# Spy container nginx from pod mypod
		kubectl spy mypod -c nginx

		# Spy container nginx from pod mypod, using busybox as the image of the spy container
		kubectl spy mypod -c nginx --spy busybox
		`
	metadataAccessor = meta.NewAccessor()
)

const (
	spyUsageStr           = "expected 'spy POD -c CONTAINER'.\nPOD is required argument for the spy command"
	defaultPodExecTimeout = 60 * time.Second
)

func NewCmdSpy(f cmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	options := &SpyOptions{
		StreamOptions: exec.StreamOptions{
			IOStreams: streams,
		},

		Executor: &DefaultRemoteExecutor{},
	}
	cmd := &cobra.Command{
		Use:                   "spy (POD | TYPE/NAME) [-c CONTAINER] [flags] -- COMMAND [args...]",
		DisableFlagsInUseLine: true,
		Short:                 "Spy a container",
		Long:                  "Spy a container",
		Example:               spyExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(options.Complete(f, cmd, args))
			cmdutil.CheckErr(options.Validate())
			cmdutil.CheckErr(options.Run(f, cmd, args))
		},
	}
	cmdutil.AddPodRunningTimeoutFlag(cmd, defaultPodExecTimeout)
	// TODO support UID
	cmd.Flags().StringVarP(&options.ContainerName, "container", "c", options.ContainerName, "Container name. If omitted, the first container in the pod will be chosen")
	options.Stdin = true
	options.TTY = true
	return cmd
}

// RemoteExecutor defines the interface accepted by the Spy command - provided for test stubbing
type RemoteExecutor interface {
	Execute(method string, url *url.URL, config *restclient.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool, terminalSizeQueue remotecommand.TerminalSizeQueue) error
}

// DefaultRemoteExecutor is the standard implementation of remote command execution
type DefaultRemoteExecutor struct{}

func (*DefaultRemoteExecutor) Execute(method string, url *url.URL, config *restclient.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool, terminalSizeQueue remotecommand.TerminalSizeQueue) error {
	spy, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}
	return spy.Stream(remotecommand.StreamOptions{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		Tty:               tty,
		TerminalSizeQueue: terminalSizeQueue,
	})
}

// SpyOptions declare the arguments accepted by the Spy command
type SpyOptions struct {
	exec.StreamOptions

	Resources []string

	ParentCommandName       string
	EnableSuggestedCmdUsage bool

	Builder          func() *resource.Builder
	ExecutablePodFn  polymorphichelpers.AttachablePodForObjectFunc
	restClientGetter genericclioptions.RESTClientGetter

	DynamicClient dynamic.Interface

	Pod           *corev1.Pod
	Executor      RemoteExecutor
	PodClient     corev1client.PodsGetter
	GetPodTimeout time.Duration
	Config        *restclient.Config
}

// Complete verifies command line arguments and loads data from the command environment
func (o *SpyOptions) Complete(f cmdutil.Factory, cmd *cobra.Command, args []string) error {
	var err error
	o.Namespace, _, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.ExecutablePodFn = polymorphichelpers.AttachablePodForObjectFn

	o.DynamicClient, err = f.DynamicClient()
	if err != nil {
		return err
	}

	o.GetPodTimeout, err = cmdutil.GetPodRunningTimeoutFlag(cmd)
	if err != nil {
		return cmdutil.UsageErrorf(cmd, err.Error())
	}

	o.Builder = f.NewBuilder
	o.Resources = args
	o.restClientGetter = f

	cmdParent := cmd.Parent()
	if cmdParent != nil {
		o.ParentCommandName = cmdParent.CommandPath()
	}
	if len(o.ParentCommandName) > 0 && cmdutil.IsSiblingCommandExists(cmd, "describe") {
		o.EnableSuggestedCmdUsage = true
	}

	config, err := f.ToRESTConfig()
	if err != nil {
		return err
	}
	o.Config = config
	return nil
}

// Validate checks that the provided spy options are specified.
func (o *SpyOptions) Validate() error {
	if len(o.Resources) == 0 {
		return fmt.Errorf("at least 1 argument is required for spy")
	}
	if len(o.Resources) > 2 {
		return fmt.Errorf("expected POD, TYPE/NAME, or TYPE NAME, (at most 2 arguments) saw %d: %v", len(o.Resources), o.Resources)
	}
	if o.GetPodTimeout <= 0 {
		return fmt.Errorf("--pod-running-timeout must be higher than zero")
	}

	return nil
}

// Run executes a validated remote execution against a pod.
func (o *SpyOptions) Run1() error {
	var err error

	if len(o.PodName) != 0 {
		// legacy pod getter
		o.Pod, err = o.PodClient.Pods(o.Namespace).Get(o.PodName, metav1.GetOptions{})
		if err != nil {
			return err
		}
	} else {
		builder := o.Builder().
			WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
			NamespaceParam(o.Namespace).DefaultNamespace()

		switch len(o.Resources) {
		case 1:
			builder.ResourceNames("pods", o.Resources[0])
		case 2:
			builder.ResourceNames(o.Resources[0], o.Resources[1])
		}

		obj, err := builder.Do().Object()
		if err != nil {
			return err
		}

		o.Pod, err = o.ExecutablePodFn(o.restClientGetter, obj, o.GetPodTimeout)

		if err != nil {
			return err
		}
	}

	pod := o.Pod

	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return fmt.Errorf("cannot spy into a container in a completed pod; current phase is %s", pod.Status.Phase) //TODO
	}

	containerName := o.ContainerName
	if len(containerName) == 0 {
		if len(pod.Spec.Containers) > 1 {
			fmt.Fprintf(o.ErrOut, "Defaulting container name to %s.\n", pod.Spec.Containers[0].Name)
			if o.EnableSuggestedCmdUsage {
				fmt.Fprintf(o.ErrOut, "Use '%s describe pod/%s -n %s' to see all of the containers in this pod.\n", o.ParentCommandName, pod.Name, o.Namespace)
			}
		}
		containerName = pod.Spec.Containers[0].Name
	}

	// ensure we can recover the terminal while attached
	t := o.SetupTTY()

	var sizeQueue remotecommand.TerminalSizeQueue
	if t.Raw {
		// this call spawns a goroutine to monitor/update the terminal size
		sizeQueue = t.MonitorSize(t.GetSize())

		// unset p.Err if it was previously set because both stdout and stderr go over p.Out when tty is
		// true
		o.ErrOut = nil
	}

	fn := func() error {
		restClient, err := restclient.RESTClientFor(o.Config)
		if err != nil {
			return err
		}

		req := restClient.Post().
			Resource("pods").
			Name(pod.Name).
			Namespace(pod.Namespace).
			SubResource("exec")
		req.VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   []string{"/bin/bash"}, //TODO: hardcoded
			Stdin:     o.Stdin,
			Stdout:    o.Out != nil,
			Stderr:    o.ErrOut != nil,
			TTY:       t.Raw,
		}, scheme.ParameterCodec)
		return o.Executor.Execute("POST", req.URL(), o.Config, o.In, o.Out, o.ErrOut, t.Raw, sizeQueue)
	}
	if err := t.Safe(fn); err != nil {
		return err
	}

	return nil
}

func (o *SpyOptions) Run(f cmdutil.Factory, cmd *cobra.Command, args []string) error {

	var err error

	if len(o.PodName) != 0 {
		// legacy pod getter
		o.Pod, err = o.PodClient.Pods(o.Namespace).Get(o.PodName, metav1.GetOptions{})
		if err != nil {
			return err
		}
	} else {
		builder := o.Builder().
			WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
			NamespaceParam(o.Namespace).DefaultNamespace()

		switch len(o.Resources) {
		case 1:
			builder.ResourceNames("pods", o.Resources[0])
		case 2:
			builder.ResourceNames(o.Resources[0], o.Resources[1])
		}

		obj, err := builder.Do().Object()
		if err != nil {
			return err
		}

		o.Pod, err = o.ExecutablePodFn(o.restClientGetter, obj, o.GetPodTimeout)

		if err != nil {
			return err
		}
	}

	pod := o.Pod

	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return fmt.Errorf("cannot spy into a container in a completed pod; current phase is %s", pod.Status.Phase) //TODO
	}

	containerName := o.ContainerName
	if len(containerName) == 0 {
		if len(pod.Spec.Containers) > 1 {
			fmt.Fprintf(o.ErrOut, "Defaulting container name to %s.\n", pod.Spec.Containers[0].Name)
			if o.EnableSuggestedCmdUsage {
				fmt.Fprintf(o.ErrOut, "Use '%s describe pod/%s -n %s' to see all of the containers in this pod.\n", o.ParentCommandName, pod.Name, o.Namespace)
			}
		}
		containerName = pod.Spec.Containers[0].Name
	}
	// TODO: run

	restartPolicy := false

	timeout, err := cmdutil.GetPodRunningTimeoutFlag(cmd)
	if err != nil {
		return cmdutil.UsageErrorf(cmd, "%v", err)
	}

	// TODO: validate spy image name
	// imageName := o.Image
	// if imageName == "" {
	// 	return fmt.Errorf("--image is required")
	// }
	// validImageRef := reference.ReferenceRegexp.MatchString(imageName)
	// if !validImageRef {
	// 	return fmt.Errorf("Invalid image name %q: %v", imageName, reference.ErrReferenceInvalidFormat)
	// }

	clientset, err := f.KubernetesClientSet()
	if err != nil {
		return err
	}

	generatorName := generateversioned.RunPodV1GeneratorName

	generators := generateversioned.GeneratorFn("run")
	generator, found := generators[generatorName]
	if !found {
		return cmdutil.UsageErrorf(cmd, "generator %q not found", generatorName)
	}

	var createdObjects = []*run.RunObject{}
	runObject, err := o.createGeneratedObject(f, cmd, generator, names, params, cmdutil.GetFlagString(cmd, "overrides"), namespace)
	if err != nil {
		return err
	}
	createdObjects = append(createdObjects, runObject)

	allErrs := []error{}

	defer o.removeCreatedObjects(f, createdObjects)

	opts := &attach.AttachOptions{
		StreamOptions: exec.StreamOptions{
			IOStreams: o.IOStreams,
			Stdin:     true,
			TTY:       true,
			// Quiet:     o.Quiet, //TODO
		},
		GetPodTimeout: timeout,
		CommandName:   cmd.Parent().CommandPath() + " attach",

		Attach: &attach.DefaultRemoteAttach{},
	}
	config, err := f.ToRESTConfig()
	if err != nil {
		return err
	}
	opts.Config = config
	opts.AttachFunc = attach.DefaultAttachFunc

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	attachablePod, err := polymorphichelpers.AttachablePodForObjectFn(f, runObject.Object, opts.GetPodTimeout)
	if err != nil {
		return err
	}
	err = handleAttachPod(f, clientset.CoreV1(), attachablePod.Namespace, attachablePod.Name, opts)
	if err != nil {
		return err
	}

	// TODO: new pod

	pod, err = waitForPod(clientset.CoreV1(), attachablePod.Namespace, attachablePod.Name, podCompleted)
	if err != nil {
		return err
	}

	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		return nil
	case corev1.PodFailed:
		unknownRcErr := fmt.Errorf("pod %s/%s failed with unknown exit code", pod.Namespace, pod.Name)
		if len(pod.Status.ContainerStatuses) == 0 || pod.Status.ContainerStatuses[0].State.Terminated == nil {
			return unknownRcErr
		}
		// assume here that we have at most one status because kubectl-run only creates one container per pod
		rc := pod.Status.ContainerStatuses[0].State.Terminated.ExitCode
		if rc == 0 {
			return unknownRcErr
		}
		return uexec.CodeExitError{
			Err:  fmt.Errorf("pod %s/%s terminated (%s)\n%s", pod.Namespace, pod.Name, pod.Status.ContainerStatuses[0].State.Terminated.Reason, pod.Status.ContainerStatuses[0].State.Terminated.Message),
			Code: int(rc),
		}
	default:
		return fmt.Errorf("pod %s/%s left in phase %s", pod.Namespace, pod.Name, pod.Status.Phase)
	}

	return nil
}

func (o *SpyOptions) removeCreatedObjects(f cmdutil.Factory, createdObjects []*run.RunObject) error {
	for _, obj := range createdObjects {
		namespace, err := metadataAccessor.Namespace(obj.Object)
		if err != nil {
			return err
		}
		var name string
		name, err = metadataAccessor.Name(obj.Object)
		if err != nil {
			return err
		}
		r := f.NewBuilder().
			WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
			ContinueOnError().
			NamespaceParam(namespace).DefaultNamespace().
			ResourceNames(obj.Mapping.Resource.Resource+"."+obj.Mapping.Resource.Group, name).
			Flatten().
			Do()

		deleteFlags := delete.NewDeleteFlags("to use to replace the resource.")
		deleteOpts := deleteFlags.ToOptions(o.DynamicClient, o.IOStreams)
		deleteOpts.IgnoreNotFound = true
		deleteOpts.WaitForDeletion = false
		deleteOpts.GracePeriod = -1
		deleteOpts.Quiet = o.Quiet

		if err := deleteOpts.DeleteResult(r); err != nil {
			return err
		}
	}

	return nil
}

func (o *SpyOptions) createGeneratedObject(f cmdutil.Factory, cmd *cobra.Command, generator generate.Generator, names []generate.GeneratorParam, params map[string]interface{}, overrides, namespace string) (*run.RunObject, error) {
	err := generate.ValidateParams(names, params)
	if err != nil {
		return nil, err
	}

	// TODO: Validate flag usage against selected generator. More tricky since --expose was added.
	obj, err := generator.Generate(params)
	if err != nil {
		return nil, err
	}

	mapper, err := f.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	// run has compiled knowledge of the thing is creating
	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.RESTMapping(gvks[0].GroupKind(), gvks[0].Version)
	if err != nil {
		return nil, err
	}

	if len(overrides) > 0 {
		codec := runtime.NewCodec(scheme.DefaultJSONEncoder(), scheme.Codecs.UniversalDecoder(scheme.Scheme.PrioritizedVersionsAllGroups()...))
		obj, err = cmdutil.Merge(codec, obj, overrides)
		if err != nil {
			return nil, err
		}
	}

	actualObj := obj

	if err := util.CreateOrUpdateAnnotation(cmdutil.GetFlagBool(cmd, cmdutil.ApplyAnnotationsFlag), obj, scheme.DefaultJSONEncoder()); err != nil {
		return nil, err
	}
	client, err := f.ClientForMapping(mapping)
	if err != nil {
		return nil, err
	}
	actualObj, err = resource.NewHelper(client, mapping).Create(namespace, false, obj, nil)
	if err != nil {
		return nil, err
	}

	return &run.RunObject{
		Object:  actualObj,
		Mapping: mapping,
	}, nil
}

// waitForPod watches the given pod until the exitCondition is true
func waitForPod(podClient corev1client.PodsGetter, ns, name string, exitCondition watchtools.ConditionFunc) (*corev1.Pod, error) {
	// TODO: expose the timeout
	ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), 0*time.Second)
	defer cancel()

	preconditionFunc := func(store cache.Store) (bool, error) {
		_, exists, err := store.Get(&metav1.ObjectMeta{Namespace: ns, Name: name})
		if err != nil {
			return true, err
		}
		if !exists {
			// We need to make sure we see the object in the cache before we start waiting for events
			// or we would be waiting for the timeout if such object didn't exist.
			// (e.g. it was deleted before we started informers so they wouldn't even see the delete event)
			return true, errors.NewNotFound(corev1.Resource("pods"), name)
		}

		return false, nil
	}

	fieldSelector := fields.OneTermEqualSelector("metadata.name", name).String()
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return podClient.Pods(ns).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return podClient.Pods(ns).Watch(options)
		},
	}

	intr := interrupt.New(nil, cancel)
	var result *corev1.Pod
	err := intr.Run(func() error {
		ev, err := watchtools.UntilWithSync(ctx, lw, &corev1.Pod{}, preconditionFunc, func(ev watch.Event) (bool, error) {
			return exitCondition(ev)
		})
		if ev != nil {
			result = ev.Object.(*corev1.Pod)
		}
		return err
	})

	return result, err
}

func handleAttachPod(f cmdutil.Factory, podClient corev1client.PodsGetter, ns, name string, opts *attach.AttachOptions) error {
	pod, err := waitForPod(podClient, ns, name, podRunningAndReady)
	if err != nil && err != ErrPodCompleted {
		return err
	}

	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return logOpts(f, pod, opts)
	}

	opts.Pod = pod
	opts.PodName = name
	opts.Namespace = ns

	if opts.AttachFunc == nil {
		opts.AttachFunc = attach.DefaultAttachFunc
	}

	if err := opts.Run(); err != nil {
		fmt.Fprintf(opts.ErrOut, "Error attaching, falling back to logs: %v\n", err)
		return logOpts(f, pod, opts)
	}
	return nil
}

// ErrPodCompleted is returned by PodRunning or PodContainerRunning to indicate that
// the pod has already reached completed state.
var ErrPodCompleted = fmt.Errorf("pod ran to completion")

// podCompleted returns true if the pod has run to completion, false if the pod has not yet
// reached running state, or an error in any other case.
func podCompleted(event watch.Event) (bool, error) {
	switch event.Type {
	case watch.Deleted:
		return false, errors.NewNotFound(schema.GroupResource{Resource: "pods"}, "")
	}
	switch t := event.Object.(type) {
	case *corev1.Pod:
		switch t.Status.Phase {
		case corev1.PodFailed, corev1.PodSucceeded:
			return true, nil
		}
	}
	return false, nil
}

// podRunningAndReady returns true if the pod is running and ready, false if the pod has not
// yet reached those states, returns ErrPodCompleted if the pod has run to completion, or
// an error in any other case.
func podRunningAndReady(event watch.Event) (bool, error) {
	switch event.Type {
	case watch.Deleted:
		return false, errors.NewNotFound(schema.GroupResource{Resource: "pods"}, "")
	}
	switch t := event.Object.(type) {
	case *corev1.Pod:
		switch t.Status.Phase {
		case corev1.PodFailed, corev1.PodSucceeded:
			return false, ErrPodCompleted
		case corev1.PodRunning:
			conditions := t.Status.Conditions
			if conditions == nil {
				return false, nil
			}
			for i := range conditions {
				if conditions[i].Type == corev1.PodReady &&
					conditions[i].Status == corev1.ConditionTrue {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// logOpts logs output from opts to the pods log.
func logOpts(restClientGetter genericclioptions.RESTClientGetter, pod *corev1.Pod, opts *attach.AttachOptions) error {
	ctrName, err := opts.GetContainerName(pod)
	if err != nil {
		return err
	}

	requests, err := polymorphichelpers.LogsForObjectFn(restClientGetter, pod, &corev1.PodLogOptions{Container: ctrName}, opts.GetPodTimeout, false)
	if err != nil {
		return err
	}
	for _, request := range requests {
		if err := logs.DefaultConsumeRequest(request, opts.Out); err != nil {
			return err
		}
	}

	return nil
}
