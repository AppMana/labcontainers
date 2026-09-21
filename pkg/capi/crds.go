package capi

// CRDs is the installable Labcontainers infrastructure-provider API. It is intentionally
// available as bytes so test harnesses do not need a source-tree-relative path.
var CRDs = []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: labclusters.infrastructure.labcontainers.appmana.com
  labels: {cluster.x-k8s.io/v1beta1: v1alpha1, cluster.x-k8s.io/v1beta2: v1alpha1}
spec:
  group: infrastructure.labcontainers.appmana.com
  scope: Namespaced
  names: {kind: LabCluster, listKind: LabClusterList, plural: labclusters, singular: labcluster}
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources: {status: {}}
      schema: {openAPIV3Schema: {type: object, x-kubernetes-preserve-unknown-fields: true}}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: labmachines.infrastructure.labcontainers.appmana.com
  labels: {cluster.x-k8s.io/v1beta1: v1alpha1, cluster.x-k8s.io/v1beta2: v1alpha1}
spec:
  group: infrastructure.labcontainers.appmana.com
  scope: Namespaced
  names: {kind: LabMachine, listKind: LabMachineList, plural: labmachines, singular: labmachine}
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources: {status: {}}
      additionalPrinterColumns:
        - {name: Node, type: string, jsonPath: .metadata.annotations.infrastructure\.labcontainers\.appmana\.com/node-name}
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties: {nodeName: {type: string}, providerID: {type: string}}
            status: {type: object, x-kubernetes-preserve-unknown-fields: true}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: labmachinetemplates.infrastructure.labcontainers.appmana.com
  labels: {cluster.x-k8s.io/v1beta1: v1alpha1, cluster.x-k8s.io/v1beta2: v1alpha1}
spec:
  group: infrastructure.labcontainers.appmana.com
  scope: Namespaced
  names: {kind: LabMachineTemplate, listKind: LabMachineTemplateList, plural: labmachinetemplates, singular: labmachinetemplate}
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema: {openAPIV3Schema: {type: object, x-kubernetes-preserve-unknown-fields: true}}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: labimportedcontrolplanes.infrastructure.labcontainers.appmana.com
  labels: {cluster.x-k8s.io/v1beta2: v1alpha1}
spec:
  group: infrastructure.labcontainers.appmana.com
  scope: Namespaced
  names: {kind: LabImportedControlPlane, listKind: LabImportedControlPlaneList, plural: labimportedcontrolplanes, singular: labimportedcontrolplane}
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources: {status: {}}
      schema: {openAPIV3Schema: {type: object, x-kubernetes-preserve-unknown-fields: true}}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: capi-aggregate-labcontainers
  labels: {cluster.x-k8s.io/aggregate-to-manager: "true"}
rules:
  - apiGroups: ["infrastructure.labcontainers.appmana.com"]
    resources: ["*"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
`)
