package cvo

import (
	"context"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	configclientv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift/cluster-version-operator/test/util"
)

var _ = g.Describe(`[Jira:"Cluster Version Operator"] cluster-version-operator works well on accept risks.`, func() {

	var (
		c              *rest.Config
		kubeClient     kubernetes.Interface
		configv1Client *configclientv1.ConfigV1Client
		err            error

		isTechPreviewNoUpgrade bool
		isHypershift           bool
		isMicroShiftCluster    bool
	)

	g.BeforeEach(func() {
		c, err = util.LoadRestConfig()
		o.Expect(err).To(o.BeNil())
		kubeClient, err = kubernetes.NewForConfig(c)
		o.Expect(err).To(o.BeNil())
		configv1Client, err = configclientv1.NewForConfig(c)
		o.Expect(err).To(o.BeNil())
		isTechPreviewNoUpgrade = util.IsTechPreviewNoUpgrade(configv1Client)
		isHypershift, err = util.IsHypershift(configv1Client)
		o.Expect(err).To(o.BeNil())
		isMicroShiftCluster, err = util.IsMicroShiftCluster(kubeClient, logger)
		o.Expect(err).To(o.BeNil())

		if isHypershift {
			g.Skip("This test is skipped on a Hypershift cluster")
		}
		if isMicroShiftCluster {
			g.Skip("This test is skipped on a Microshift cluster")
		}
		if !isTechPreviewNoUpgrade {
			g.Skip("This test is skipped because the Tech Preview NoUpgrade is not enabled")
		}
	})

	g.Describe("Cluster Version Operator", func() {
		g.It("should populate the fields about risks in status", func() {
			cv, err := configv1Client.ClusterVersions().Get(context.Background(), "version", metav1.GetOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Checking that each condition update has risk names")
			for _, cu := range cv.Status.ConditionalUpdates {
				o.Expect(cu.RiskNames).ShouldNot(o.BeEmpty(), "RiskNames should not be empty")
			}
		})
	})
})
