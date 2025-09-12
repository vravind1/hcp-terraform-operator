// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"fmt"
	"time"

	tfc "github.com/hashicorp/go-tfe"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TestObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (in *TestObject) DeepCopyObject() runtime.Object {
	return nil
}

var _ = Describe("Helpers", Label("Unit"), func() {
	Context("Returns", func() {
		It("do not requeue", func() {
			result, err := doNotRequeue()
			Expect(result).To(BeEquivalentTo(reconcile.Result{}))
			Expect(err).To(BeNil())
		})
		It("requeue after", func() {
			duration := 1 * time.Second
			result, err := requeueAfter(duration)
			Expect(result).To(BeEquivalentTo(reconcile.Result{Requeue: true, RequeueAfter: duration}))
			Expect(err).To(BeNil())
		})
		It("requeue on error", func() {
			result, err := requeueOnErr(fmt.Errorf(""))
			Expect(result).To(BeEquivalentTo(reconcile.Result{}))
			Expect(err).ToNot(BeNil())
		})
	})

	Context("FormatOutput", func() {
		It("bool", func() {
			o := &tfc.StateVersionOutput{
				Type:  "boolean",
				Value: true,
			}
			e := "true"
			result, err := formatOutput(o)
			Expect(result).To(BeEquivalentTo(e))
			Expect(err).To(BeNil())
		})
		It("string", func() {
			o := &tfc.StateVersionOutput{
				Type:  "string",
				Value: "hello",
			}
			e := "hello"
			result, err := formatOutput(o)
			Expect(result).To(BeEquivalentTo(e))
			Expect(err).To(BeNil())
		})
		It("multilineString", func() {
			o := &tfc.StateVersionOutput{
				Type:  "string",
				Value: "hello\nworld",
			}
			e := "hello\nworld"
			result, err := formatOutput(o)
			Expect(result).To(BeEquivalentTo(e))
			Expect(err).To(BeNil())
		})
		It("number", func() {
			o := &tfc.StateVersionOutput{
				Type:  "number",
				Value: 162,
			}
			e := "162"
			result, err := formatOutput(o)
			Expect(result).To(BeEquivalentTo(e))
			Expect(err).To(BeNil())
		})
		It("list", func() {
			o := &tfc.StateVersionOutput{
				Type: "array",
				Value: []any{
					"one",
					2,
				},
			}
			e := `["one",2]`
			result, err := formatOutput(o)
			Expect(result).To(BeEquivalentTo(e))
			Expect(err).To(BeNil())
		})
		It("map", func() {
			o := &tfc.StateVersionOutput{
				Type: "array",
				Value: map[string]string{
					"one": "een",
					"two": "twee",
				},
			}
			e := `{"one":"een","two":"twee"}`
			result, err := formatOutput(o)
			Expect(result).To(BeEquivalentTo(e))
			Expect(err).To(BeNil())
		})
	})

	Context("NeedToAddFinalizer", func() {
		testFinalizer := "test.app.terraform.io/finalizer"
		o := TestObject{}
		It("No deletion timestamp and no finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = nil
			o.ObjectMeta.Finalizers = []string{}
			Expect(needToAddFinalizer(&o, testFinalizer)).To(BeTrue())
		})
		It("No deletion timestamp and finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = nil
			o.ObjectMeta.Finalizers = []string{testFinalizer}
			Expect(needToAddFinalizer(&o, testFinalizer)).To(BeFalse())
		})
		It("Deletion timestamp and no finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			o.ObjectMeta.Finalizers = []string{}
			Expect(needToAddFinalizer(&o, testFinalizer)).To(BeFalse())
		})
		It("Deletion timestamp and finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			o.ObjectMeta.Finalizers = []string{testFinalizer}
			Expect(needToAddFinalizer(&o, testFinalizer)).To(BeFalse())
		})
	})

	Context("IsDeletionCandidate", func() {
		testFinalizer := "test.app.terraform.io/finalizer"
		o := TestObject{}
		It("No deletion timestamp and no finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = nil
			o.ObjectMeta.Finalizers = []string{}
			Expect(isDeletionCandidate(&o, testFinalizer)).To(BeFalse())
		})
		It("No deletion timestamp and finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = nil
			o.ObjectMeta.Finalizers = []string{testFinalizer}
			Expect(isDeletionCandidate(&o, testFinalizer)).To(BeFalse())
		})
		It("Deletion timestamp and no finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			o.ObjectMeta.Finalizers = []string{}
			Expect(isDeletionCandidate(&o, testFinalizer)).To(BeFalse())
		})
		It("Deletion timestamp and finalizer", func() {
			o.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			o.ObjectMeta.Finalizers = []string{testFinalizer}
			Expect(isDeletionCandidate(&o, testFinalizer)).To(BeTrue())
		})
	})

	Context("Match wildcard name", func() {
		// True
		It("match prefix", func() {
			result := matchWildcardName("*-terraform-workspace", "hcp-terraform-workspace")
			Expect(result).To(BeTrue())
		})
		It("match suffix", func() {
			result := matchWildcardName("hcp-terraform-*", "hcp-terraform-workspace")
			Expect(result).To(BeTrue())
		})
		It("match prefix and suffix", func() {
			result := matchWildcardName("*-terraform-*", "hcp-terraform-workspace")
			Expect(result).To(BeTrue())
		})
		It("match no prefix and no suffix", func() {
			result := matchWildcardName("hcp-terraform-workspace", "hcp-terraform-workspace")
			Expect(result).To(BeTrue())
		})
		// False
		It("does not match prefix", func() {
			result := matchWildcardName("*-terraform-workspace", "hcp-tf-workspace")
			Expect(result).To(BeFalse())
		})
		It("does not match suffix", func() {
			result := matchWildcardName("hcp-terraform-*", "hashicorp-tf-workspace")
			Expect(result).To(BeFalse())
		})
		It("does not match prefix and suffix", func() {
			result := matchWildcardName("*-terraform-*", "hcp-tf-workspace")
			Expect(result).To(BeFalse())
		})
		It("does not match no prefix and no suffix", func() {
			result := matchWildcardName("hcp-terraform-workspace", "hcp-tf-workspace")
			Expect(result).To(BeFalse())
		})
	})

	Context("ParseTFEVersion", func() {
		It("Valid TFE version", func() {
			version := "v202502-1"
			v, err := parseTFEVersion(version)
			Expect(err).To(Succeed())
			Expect(v).To(Equal(2025021))
		})
		It("Invalid TFE version", func() {
			version := "202502-1"
			_, err := parseTFEVersion(version)
			Expect(err).ToNot(Succeed())
		})
		It("Empty TFE version", func() {
			version := ""
			_, err := parseTFEVersion(version)
			Expect(err).ToNot(Succeed())
		})
		It("Semantic version compatibility", func() {
			version := "1.0.0"
			v, err := parseTFEVersion(version)
			Expect(err).To(Succeed())
			Expect(v).To(Equal(301000000)) // semanticVersionBase + 1*1_000_000 + 0*1_000 + 0
		})
	})

	Context("ParseTFEVersionDetailed", func() {
		Context("Legacy format", func() {
			DescribeTable("Valid legacy versions",
				func(version string, expectedNum int) {
					versionNum, isSemantic, err := parseTFEVersionDetailed(version)
					Expect(err).To(Succeed())
					Expect(isSemantic).To(BeFalse())
					Expect(versionNum).To(Equal(expectedNum))
				},
				Entry("Future version", "v202502-1", 2025021),
				Entry("Boundary version", "v202409-1", 2024091),
				Entry("Before threshold", "v202408-1", 2024081),
				Entry("Old version", "v202012-5", 2020125),
			)

			DescribeTable("Invalid legacy versions",
				func(version string) {
					_, _, err := parseTFEVersionDetailed(version)
					Expect(err).To(HaveOccurred())
				},
				Entry("Missing v prefix", "202502-1"),
				Entry("Invalid date format", "v20250-1"),
				Entry("Two-digit suffix", "v202409-10"),
			)
		})

		Context("Semantic format", func() {
			DescribeTable("Valid semantic versions",
				func(version string, expectedMajor, expectedMinor, expectedPatch int) {
					versionNum, isSemantic, err := parseTFEVersionDetailed(version)
					Expect(err).To(Succeed())
					Expect(isSemantic).To(BeTrue())

					// Verify encoding
					expectedEncoded := 300000000 + expectedMajor*1_000_000 + expectedMinor*1_000 + expectedPatch
					Expect(versionNum).To(Equal(expectedEncoded))

					// Verify it's above semantic threshold
					Expect(versionNum).To(BeNumerically(">=", 300000000))
				},
				Entry("Simple version", "1.0.0", 1, 0, 0),
				Entry("Minor version", "1.1.0", 1, 1, 0),
				Entry("Patch version", "2.0.3", 2, 0, 3),
				Entry("Large numbers", "10.12.5", 10, 12, 5),
				Entry("With prerelease", "1.0.0-alpha", 1, 0, 0),
				Entry("Complex prerelease", "2.1.3-beta.1", 2, 1, 3),
				Entry("With build metadata", "1.0.0+build.123", 1, 0, 0),
				Entry("Prerelease and build", "1.2.3-alpha+build", 1, 2, 3),
			)

			DescribeTable("Invalid semantic versions",
				func(version string) {
					_, _, err := parseTFEVersionDetailed(version)
					Expect(err).To(HaveOccurred())
				},
				Entry("Incomplete version", "1.0"),
				Entry("Too many parts", "1.0.0.1"),
				Entry("Non-numeric major", "a.0.0"),
				Entry("Non-numeric minor", "1.b.0"),
				Entry("Non-numeric patch", "1.0.c"),
			)
		})

		Context("Malformed versions", func() {
			DescribeTable("Invalid versions",
				func(version string) {
					_, _, err := parseTFEVersionDetailed(version)
					Expect(err).To(HaveOccurred())
				},
				Entry("Empty string", ""),
				Entry("Random text", "foo"),
				Entry("Just numbers", "123456"),
			)
		})

		Context("Algorithm selection logic", func() {
			DescribeTable("New algorithm selection",
				func(version string, shouldUseNewAlgorithm bool) {
					versionNum, isSemantic, err := parseTFEVersionDetailed(version)
					Expect(err).To(Succeed())

					// Logic: isSemantic || versionNum >= legacyVersionThreshold
					usesNewAlgorithm := isSemantic || versionNum >= 2024091
					Expect(usesNewAlgorithm).To(Equal(shouldUseNewAlgorithm))
				},
				Entry("Semantic version always uses new", "1.0.0", true),
				Entry("Legacy at threshold uses new", "v202409-1", true),
				Entry("Legacy above threshold uses new", "v202502-1", true),
				Entry("Legacy below threshold uses old", "v202408-1", false),
				Entry("Old legacy version uses old", "v202012-5", false),
			)
		})
	})
})
