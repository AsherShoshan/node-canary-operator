# node-canary-operator
Manage canary pod post a node drain

Cordon a node and Draining a node results with the same taint: node.kubernetes.io/unschedulable=:NoSchedule
Taking actions, when only a drain occured, can't be done with this taint.

This small operator, maintains a 'canary' deployment, on each node, and watches nodes for cordon/drain action.
If it was a drain, then this 'canary' pod is evicted, and the operator will add additional taint ("kubevirt.io/drain"), 
so other operators can watch this special taint, and start drain implied actions.  For example live migration of Kubevirt virtualmachine instances.

Master nodes, are also supported, once the master node becomes schedulable. i.e (taint master node-role.kubernetes.io/master-)


# Installation:

kubectl apply -f https://github.com/AsherShoshan/node-canary-operator/blob/master/deploy/service_account.yaml

kubectl apply -f https://github.com/AsherShoshan/node-canary-operator/blob/master/deploy/role.yaml

kubectl apply -f https://github.com/AsherShoshan/node-canary-operator/blob/master/deploy/role_binding.yaml

kubectl apply -f https://github.com/AsherShoshan/node-canary-operator/blob/master/deploy/operator.yaml


Make sure to adjust namespace value in service_account.yaml, role_binding.yaml, and "WATCH_NAMESPACE" in operator.yaml
