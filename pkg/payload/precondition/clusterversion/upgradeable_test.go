package clusterversion

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift/cluster-version-operator/pkg/payload/precondition"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	configv1listers "github.com/openshift/client-go/config/listers/config/v1"

	"k8s.io/client-go/tools/cache"
)

func TestGetEffectiveMinor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "invalid",
			input:    "something@very-differe",
			expected: "",
		},
		{
			name:     "multidot",
			input:    "v4.7.12.3+foo",
			expected: "7",
		},
		{
			name:     "single",
			input:    "v4.7",
			expected: "7",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := GetEffectiveMinor(tc.input)
			if tc.expected != actual {
				t.Error(actual)
			}
		})
	}
}

func TestUpgradeableRun(t *testing.T) {
	ctx := context.Background()
	ptr := func(status configv1.ConditionStatus) *configv1.ConditionStatus {
		return &status
	}

	tests := []struct {
		name               string
		upgradeable        *configv1.ConditionStatus
		currVersion        string
		desiredVersion     string
		desiredVersionInCV string
		upgradeInProgress  bool
		expected           string
	}{
		{
			name:           "first",
			desiredVersion: "4.2",
			expected:       "",
		},
		{
			name:           "move-y, no condition",
			currVersion:    "4.1",
			desiredVersion: "4.2",
			expected:       "",
		},
		{
			name:           "move-y, with true condition",
			upgradeable:    ptr(configv1.ConditionTrue),
			currVersion:    "4.1",
			desiredVersion: "4.2",
			expected:       "",
		},
		{
			name:           "move-y, with unknown condition",
			upgradeable:    ptr(configv1.ConditionUnknown),
			currVersion:    "4.1",
			desiredVersion: "4.2",
			expected:       "",
		},
		{
			name:           "move-y, with false condition",
			upgradeable:    ptr(configv1.ConditionFalse),
			currVersion:    "4.1",
			desiredVersion: "4.2",
			expected:       "set to False",
		},
		{
			name:           "move-z, with false condition",
			upgradeable:    ptr(configv1.ConditionFalse),
			currVersion:    "4.1.3",
			desiredVersion: "4.1.4",
			expected:       "",
		},
		{
			name:               "move-(y+1) while move-y is in progress",
			currVersion:        "4.6.3",
			desiredVersionInCV: "4.7.2",
			desiredVersion:     "4.8.1",
			upgradeInProgress:  true,
			expected:           "The minor level upgrade to 4.8.1 is not recommended: UpgradeInProgress y to y+1. It is recommended to wait until the existing upgrade completes.",
		},
		{
			name:               "move-(y+1) while move-z is in progress",
			currVersion:        "4.14.15",
			desiredVersionInCV: "4.14.35",
			desiredVersion:     "4.15.29",
			upgradeInProgress:  true,
			expected:           "The minor level upgrade to 4.15.29 is not recommended: UpgradeInProgress y to y+1. It is recommended to wait until the existing upgrade completes.",
		},
		{
			name:               "move-y with z while move-y is in progress",
			currVersion:        "4.6.3",
			desiredVersionInCV: "4.7.2",
			upgradeInProgress:  true,
			desiredVersion:     "4.7.3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clusterVersion := &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec:       configv1.ClusterVersionSpec{},
				Status: configv1.ClusterVersionStatus{
					History: []configv1.UpdateHistory{},
					Desired: configv1.Release{Version: tc.desiredVersionInCV},
				},
			}
			if len(tc.currVersion) > 0 {
				clusterVersion.Status.History = append(clusterVersion.Status.History, configv1.UpdateHistory{Version: tc.currVersion, State: configv1.CompletedUpdate})
			}
			if tc.upgradeable != nil {
				clusterVersion.Status.Conditions = append(clusterVersion.Status.Conditions, configv1.ClusterOperatorStatusCondition{
					Type:    configv1.OperatorUpgradeable,
					Status:  *tc.upgradeable,
					Message: fmt.Sprintf("set to %v", *tc.upgradeable),
				})
			}
			if tc.upgradeInProgress {
				clusterVersion.Status.Conditions = append(clusterVersion.Status.Conditions, configv1.ClusterOperatorStatusCondition{
					Type:    UpgradeInProgress,
					Status:  configv1.ConditionTrue,
					Message: "UpgradeInProgress y to y+1.",
				})
			}
			cvLister := fakeClusterVersionLister(t, clusterVersion)
			instance := NewUpgradeable(cvLister)

			err := instance.Run(ctx, precondition.ReleaseContext{
				DesiredVersion: tc.desiredVersion,
			})
			switch {
			case err != nil && len(tc.expected) == 0:
				t.Error(err)
			case err != nil && err.Error() == tc.expected:
			case err != nil && err.Error() != tc.expected:
				t.Error(err)
			case err == nil && len(tc.expected) == 0:
			case err == nil && len(tc.expected) != 0:
				t.Error(err)
			}

		})
	}
}

func fakeClusterVersionLister(t *testing.T, clusterVersion *configv1.ClusterVersion) configv1listers.ClusterVersionLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if clusterVersion == nil {
		return configv1listers.NewClusterVersionLister(indexer)
	}

	err := indexer.Add(clusterVersion)
	if err != nil {
		t.Fatal(err)
	}
	return configv1listers.NewClusterVersionLister(indexer)
}
