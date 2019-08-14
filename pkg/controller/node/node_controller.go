package node

import (
	"context"
	"os"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_node")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Node Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNode{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("node-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	chck := func(obj runtime.Object) bool {
		node := obj.DeepCopyObject().(*corev1.Node)
		for _, taint := range node.Spec.Taints {
			if taint.Effect == "NoSchedule" && taint.Key != "kubevirt.io/drain" {
				return true
			}
		}
		return false
	}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return chck(e.ObjectOld) != chck(e.ObjectNew) //return xor - if changed old <-> new
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Deployment requeue the owner Node
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &corev1.Node{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNode implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNode{}

// ReconcileNode reconciles a Node object
type ReconcileNode struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CnvPod object and makes changes based on the state read
// and what is in the CnvPod.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNode) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	reqLogger := log.WithValues("Namespace", request.Namespace, "Name", request.Name)
	reqLogger.Info("Reconciling Node")

	// Fetch the Node instance
	node := &corev1.Node{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: request.Name, Namespace: ""}, node)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Create a new Deployment struct
	dep, err := r.newDepForNode(node)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Check if the Deployment exists  (Deployment-name = canary-<node-name>)
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: dep.Name, Namespace: dep.Namespace}, dep)
	if err != nil {
		if errors.IsNotFound(err) { //Deployment not found

			// do not create deployment in case node is "NoSchedule"
			for _, taint := range node.Spec.Taints {
				if taint.Effect == "NoSchedule" {
					return reconcile.Result{}, nil
				}
			}

			// Create the Deployment
			reqLogger.Info("Creating Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			if err = r.client.Create(context.TODO(), dep); err != nil {
				return reconcile.Result{}, err
			}

		} else { //other error
			return reconcile.Result{}, err
		}
	}

	// Search taints
	taintCordonExists := false //node cordoned or drained
	taintDrainExists := false  //if special taint is there
	taintDraini := 0
	for i, taint := range node.Spec.Taints {
		if taint.Key == "node.kubernetes.io/unschedulable" {
			taintCordonExists = true
		}
		if taint.Key == "kubevirt.io/drain" {
			taintDrainExists = true
			taintDraini = i
		}
	}

	updateNode := false
	updateMsg := ""
	// if pod evicted than it means node drained - add special taint
	if taintCordonExists && dep.Status.ReadyReplicas == 0 && !taintDrainExists {
		// Update node with special taint
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    "kubevirt.io/drain",
			Value:  "draining",
			Effect: "NoSchedule",
		})
		updateNode = true
		updateMsg = "Added"
	}
	// if node uncordoned - remove special taint
	if !taintCordonExists && taintDrainExists {
		node.Spec.Taints[taintDraini] = node.Spec.Taints[len(node.Spec.Taints)-1]
		node.Spec.Taints = node.Spec.Taints[:len(node.Spec.Taints)-1]
		updateNode = true
		updateMsg = "Removed"
	}
	if updateNode {
		reqLogger.Info("Taint 'kubevirt.io/drain' - "+updateMsg, "Node", node.Name)
		if err = r.client.Update(context.TODO(), node); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// newDepForNode returns a Deployment with the "canary-"<name> as the Node
func (r *ReconcileNode) newDepForNode(node *corev1.Node) (*appsv1.Deployment, error) {

	depNs := os.Getenv("WATCH_NAMESPACE")
	if depNs == "" {
		// Get the namespace the operator is currently deployed in.
		depNs, _ = k8sutil.GetOperatorNamespace()
	}

	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-canary-" + node.Name,
			Namespace: depNs,
		},
		Spec: appsv1.DeploymentSpec{
			//Replicas: int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "node-canary"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "node-canary"},
				},
				Spec: corev1.PodSpec{
					NodeSelector: map[string]string{"kubernetes.io/hostname": node.Name},
					Containers: []corev1.Container{{
						Image:   "busybox",
						Name:    "busybox",
						Command: []string{"bin/sh"},
						Args:    []string{"-c", "while true; do sleep 3600; done"},
					}},
				},
			},
		},
	}

	// Set Node as the owner and controller of Deployment
	err := controllerutil.SetControllerReference(node, dep, r.scheme)
	return dep, err
}
