/*
Copyright 2026 Joshua Mathianas.

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

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	virtv1alpha1 "github.com/mathianasj/virt-git-sync/api/v1alpha1"
)

var _ = Describe("VirtGitSync Controller", func() {
	Context("When reconciling a resource", func() {
		It("should successfully reconcile and add finalizer", func() {
			resourceName := fmt.Sprintf("test-resource-%d", time.Now().UnixNano())
			ctx := context.Background()

			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			By("creating the custom resource for the Kind VirtGitSync")
			resource := &virtv1alpha1.VirtGitSync{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: virtv1alpha1.VirtGitSyncSpec{},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			defer func() {
				By("Cleanup the specific resource instance VirtGitSync")
				resource := &virtv1alpha1.VirtGitSync{}
				_ = k8sClient.Get(ctx, typeNamespacedName, resource)
				_ = k8sClient.Delete(ctx, resource)
			}()

			By("Reconciling the created resource")
			controllerReconciler := &VirtGitSyncReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile updates status
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if finalizer was added and status updated")
			virtgitsync := &virtv1alpha1.VirtGitSync{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, virtgitsync)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(virtgitsync, virtGitSyncFinalizer)).To(BeTrue())
			Expect(virtgitsync.Status.Phase).To(Equal(virtv1alpha1.VirtGitSyncPhaseRunning))
		})
	})

	Context("YAML Cleaning for GitOps", func() {
		It("should remove system-managed annotations", func() {
			running := false
			memory := resource.MustParse("128Mi")
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					Annotations: map[string]string{
						"my-custom-annotation":                             "keep-this",
						"kubectl.kubernetes.io/last-applied-configuration": "remove-this",
						"kubemacpool.io/transaction-timestamp":             "remove-this",
						"kubevirt.io/latest-observed-api-version":          "remove-this",
						"kubernetes.io/service-account.name":               "remove-this",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Running: &running,
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Domain: kubevirtv1.DomainSpec{
								Resources: kubevirtv1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: memory,
									},
								},
							},
						},
					},
				},
			}

			cleanVM := cleanVMForGitOps(vm)

			// Convert to YAML and check
			yamlData, err := yaml.Marshal(cleanVM)
			Expect(err).NotTo(HaveOccurred())
			yamlStr := string(yamlData)

			// Should keep custom annotation
			Expect(yamlStr).To(ContainSubstring("my-custom-annotation: keep-this"))

			// Should remove system annotations
			Expect(yamlStr).NotTo(ContainSubstring("kubectl.kubernetes.io"))
			Expect(yamlStr).NotTo(ContainSubstring("kubemacpool.io"))
			Expect(yamlStr).NotTo(ContainSubstring("latest-observed-api-version"))
			Expect(yamlStr).NotTo(ContainSubstring("service-account.name"))
		})

		It("should remove system-managed labels", func() {
			running := false
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					Labels: map[string]string{
						"app":                             "myapp",
						"tier":                            "backend",
						"kubernetes.io/metadata.name":     "remove-this",
						"k8s.io/cluster-name":             "remove-this",
						"openshift.io/cluster-monitoring": "remove-this",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Running: &running,
				},
			}

			cleanVM := cleanVMForGitOps(vm)

			yamlData, err := yaml.Marshal(cleanVM)
			Expect(err).NotTo(HaveOccurred())
			yamlStr := string(yamlData)

			// Should keep user labels
			Expect(yamlStr).To(ContainSubstring("app: myapp"))
			Expect(yamlStr).To(ContainSubstring("tier: backend"))

			// Should remove system labels
			Expect(yamlStr).NotTo(ContainSubstring("kubernetes.io"))
			Expect(yamlStr).NotTo(ContainSubstring("k8s.io"))
			Expect(yamlStr).NotTo(ContainSubstring("openshift.io"))
		})

		It("should include required fields for Argo CD compatibility", func() {
			running := false
			memory := resource.MustParse("64Mi")
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Running: &running,
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"kubevirt.io/pci-topology-version": "v3",
							},
						},
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Architecture: "amd64",
							Domain: kubevirtv1.DomainSpec{
								Firmware: &kubevirtv1.Firmware{
									Serial: "test-serial",
									UUID:   "test-uuid",
								},
								Machine: &kubevirtv1.Machine{
									Type: "pc-q35-rhel9.6.0",
								},
								Resources: kubevirtv1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: memory,
									},
								},
							},
						},
					},
				},
			}

			cleanVM := cleanVMForGitOps(vm)

			yamlData, err := yaml.Marshal(cleanVM)
			Expect(err).NotTo(HaveOccurred())
			yamlStr := string(yamlData)

			// Should include fields needed for Argo CD compatibility
			Expect(yamlStr).To(ContainSubstring("apiVersion: kubevirt.io/v1"))
			Expect(yamlStr).To(ContainSubstring("kind: VirtualMachine"))
			Expect(yamlStr).To(ContainSubstring("architecture: amd64"))
			Expect(yamlStr).To(ContainSubstring("kubevirt.io/pci-topology-version: v3"))
			Expect(yamlStr).To(ContainSubstring("creationTimestamp: null"))
			Expect(yamlStr).To(ContainSubstring("serial: test-serial"))
			Expect(yamlStr).To(ContainSubstring("uuid: test-uuid"))
			Expect(yamlStr).To(ContainSubstring("type: pc-q35-rhel9.6.0"))
		})

		It("should not include runtime metadata", func() {
			running := false
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-vm",
					Namespace:         "default",
					ResourceVersion:   "12345",
					UID:               "test-uid-123",
					Generation:        5,
					CreationTimestamp: metav1.Now(),
					Finalizers:        []string{"test-finalizer"},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Running: &running,
				},
			}

			cleanVM := cleanVMForGitOps(vm)

			yamlData, err := yaml.Marshal(cleanVM)
			Expect(err).NotTo(HaveOccurred())
			yamlStr := string(yamlData)

			// Should NOT include runtime fields
			Expect(yamlStr).NotTo(ContainSubstring("resourceVersion"))
			Expect(yamlStr).NotTo(ContainSubstring("uid"))
			Expect(yamlStr).NotTo(ContainSubstring("generation"))
			Expect(yamlStr).NotTo(ContainSubstring("finalizers"))

			// Metadata should only have name and namespace
			Expect(yamlStr).To(ContainSubstring("name: test-vm"))
			Expect(yamlStr).To(ContainSubstring("namespace: default"))
		})
	})

	Context("System Annotation and Label Filtering", func() {
		It("should correctly identify system-managed annotations", func() {
			systemAnnotations := []string{
				"kubectl.kubernetes.io/last-applied-configuration",
				"kubemacpool.io/transaction-timestamp",
				"kubevirt.io/latest-observed-api-version",
				"kubernetes.io/service-account.name",
				"openshift.io/something",
				"pv.kubernetes.io/bound-by-controller",
			}

			for _, annotation := range systemAnnotations {
				Expect(isSystemManagedAnnotation(annotation)).To(BeTrue(),
					fmt.Sprintf("%s should be identified as system-managed", annotation))
			}

			userAnnotations := []string{
				"my-custom-annotation",
				"app.example.com/version",
				"team/owner",
			}

			for _, annotation := range userAnnotations {
				Expect(isSystemManagedAnnotation(annotation)).To(BeFalse(),
					fmt.Sprintf("%s should NOT be identified as system-managed", annotation))
			}
		})

		It("should correctly identify system-managed labels", func() {
			systemLabels := []string{
				"kubernetes.io/metadata.name",
				"k8s.io/cluster-name",
				"openshift.io/cluster-monitoring",
				"app.kubernetes.io/managed-by",
			}

			for _, label := range systemLabels {
				Expect(isSystemManagedLabel(label)).To(BeTrue(),
					fmt.Sprintf("%s should be identified as system-managed", label))
			}

			userLabels := []string{
				"app",
				"tier",
				"environment",
				"app.example.com/component",
			}

			for _, label := range userLabels {
				Expect(isSystemManagedLabel(label)).To(BeFalse(),
					fmt.Sprintf("%s should NOT be identified as system-managed", label))
			}
		})
	})

	Context("Auto-Pause/Unpause Functionality", func() {
		var handler *VirtualMachineEventHandler

		BeforeEach(func() {
			handler = &VirtualMachineEventHandler{
				Client: k8sClient,
			}
		})

		It("should auto-pause when manual change is detected", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways
			runStrategyHalted := kubevirtv1.RunStrategyHalted

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-autopause",
					Namespace: "default",
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "old-config",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyHalted,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-autopause",
					Namespace: "default",
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "old-config", // NOT updated
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways, // Manual change
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			shouldPause := handler.shouldAutoPause(oldVM, newVM, changes)

			Expect(shouldPause).To(BeTrue(), "Should auto-pause for manual runStrategy change")
		})

		It("should NOT auto-pause when ArgoCD makes the change", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways
			runStrategyHalted := kubevirtv1.RunStrategyHalted

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-argo",
					Namespace: "default",
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "old-config",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyHalted,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-argo",
					Namespace: "default",
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "new-config", // ArgoCD updated this
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			shouldPause := handler.shouldAutoPause(oldVM, newVM, changes)

			Expect(shouldPause).To(BeFalse(), "Should NOT auto-pause for ArgoCD change")
		})

		It("should NOT auto-pause when VM is already paused", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways
			runStrategyHalted := kubevirtv1.RunStrategyHalted

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-already-paused",
					Namespace: "default",
					Annotations: map[string]string{
						PauseArgoAnnotation: "true",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyHalted,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-already-paused",
					Namespace: "default",
					Annotations: map[string]string{
						PauseArgoAnnotation: "true",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			shouldPause := handler.shouldAutoPause(oldVM, newVM, changes)

			Expect(shouldPause).To(BeFalse(), "Should NOT auto-pause if already paused")
		})

		It("should NOT auto-pause when ONLY pause annotation is removed", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-unpause",
					Namespace: "default",
					Annotations: map[string]string{
						PauseArgoAnnotation: "true",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-vm-unpause",
					Namespace:   "default",
					Annotations: map[string]string{
						// pause annotation removed
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways, // No other changes
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			shouldPause := handler.shouldAutoPause(oldVM, newVM, changes)

			Expect(shouldPause).To(BeFalse(), "Should NOT re-add pause when user removes it")
		})

		It("should NOT auto-pause when only resourceVersion changes", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-vm-resourceversion",
					Namespace:       "default",
					ResourceVersion: "12345",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-vm-resourceversion",
					Namespace:       "default",
					ResourceVersion: "12346", // Only this changed
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			shouldPause := handler.shouldAutoPause(oldVM, newVM, changes)

			Expect(shouldPause).To(BeFalse(), "Should NOT auto-pause for resourceVersion-only change")
		})

		It("should detect runStrategy changes", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways
			runStrategyHalted := kubevirtv1.RunStrategyHalted

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-runstrategy",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyHalted,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-runstrategy",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			Expect(changes).To(HaveKey("spec.runStrategy"), "Should detect runStrategy change")
			change := changes["spec.runStrategy"].(map[string]interface{})
			Expect(change["old"]).To(Equal(runStrategyHalted))
			Expect(change["new"]).To(Equal(runStrategyAlways))
		})

		It("should detect label additions", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-labels",
					Namespace: "default",
					Labels: map[string]string{
						"app": "myapp",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-labels",
					Namespace: "default",
					Labels: map[string]string{
						"app":        "myapp",
						"new-label":  "new-value",
						"test-label": "test-value",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			Expect(changes).To(HaveKey("metadata.labels"), "Should detect label changes")
		})

		It("should auto-pause for label changes (manual)", func() {
			runStrategyAlways := kubevirtv1.RunStrategyAlways

			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-manual-label",
					Namespace: "default",
					Labels: map[string]string{
						"app": "myapp",
					},
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "old-config",
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-manual-label",
					Namespace: "default",
					Labels: map[string]string{
						"app":         "myapp",
						"manual-edit": "true",
					},
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "old-config", // NOT updated by ArgoCD
					},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			shouldPause := handler.shouldAutoPause(oldVM, newVM, changes)

			Expect(shouldPause).To(BeTrue(), "Should auto-pause for manual label change")
		})

		It("should handle nil runStrategy", func() {
			oldVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-nil-runstrategy",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: nil,
				},
			}

			runStrategyAlways := kubevirtv1.RunStrategyAlways
			newVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm-nil-runstrategy",
					Namespace: "default",
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					RunStrategy: &runStrategyAlways,
				},
			}

			changes := handler.detectChanges(oldVM, newVM)
			Expect(changes).To(HaveKey("spec.runStrategy"), "Should detect nil to non-nil runStrategy change")
		})

		It("should correctly identify paused VMs", func() {
			pausedVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "paused-vm",
					Namespace: "default",
					Annotations: map[string]string{
						PauseArgoAnnotation: "true",
					},
				},
			}

			notPausedVM := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-paused-vm",
					Namespace: "default",
				},
			}

			Expect(isVMPaused(pausedVM)).To(BeTrue(), "VM with pause annotation should be paused")
			Expect(isVMPaused(notPausedVM)).To(BeFalse(), "VM without pause annotation should not be paused")
		})
	})
})
