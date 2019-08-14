# node-canary-operator
Manage canary pod post a node drain

Cordon a node and Draining a node results with the same taint: node.kubernetes.io/unschedulable=:NoSchedule
Taking actions, when only a drain occured, can't be done with this taint.

This small operator, maintains a 'canary' deployment, on each node, and watches nodes for cordon/drain action.
If it was a drain, then this 'canary' pod is evicted, and the operator will add additional taint ("kubevirt.io/drain"), 
so other operators can watch this special taint, and start drain implied actions.  For example live migration of Kubevirt virtualmachine instances.

Master nodes, are also supported, once the master node becomes schedulable. i.e (taint master node-role.kubernetes.io/master-)


Install
-------
export TARGET_NAMESPACE=your-target-namespace     (default to openshift-operators)

curl -k https://raw.githubusercontent.com/AsherShoshan/node-canary-operator/master/deploy.sh | bash

Uninstall
---------
export TARGET_NAMESPACE=your-target-namespace     (default to openshift-operators)

curl -k https://raw.githubusercontent.com/AsherShoshan/node-canary-operator/master/undeploy.sh | bash
