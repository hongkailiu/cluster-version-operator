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
		name                     string
		upgradeable              *configv1.ConditionStatus
		currVersion              string
		desiredVersion           string
		versionPartiallyUpgraded string
		NonBlockingWarning       bool
		expected                 string
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
			name:                     "move-(y+1) while move-y is in progress",
			currVersion:              "4.6.3",
			versionPartiallyUpgraded: "4.7.2",
			desiredVersion:           "4.8.1",
			expected:                 "The minor level upgrade to 4.8.1 is not recommended until the existing upgrade from 4.6.3 to 4.7.2 completes.",
		},
		{
			name:                     "move-(y+1) while move-z is in progress",
			currVersion:              "4.14.15",
			versionPartiallyUpgraded: "4.14.35",
			desiredVersion:           "4.15.29",
			expected:                 "The minor level upgrade to 4.15.29 is not recommended until the existing upgrade from 4.14.15 to 4.14.35 completes.",
		},
		{
			name:                     "move-y with z while move-y is in progress",
			currVersion:              "4.6.3",
			versionPartiallyUpgraded: "4.7.2",
			desiredVersion:           "4.7.3",
			NonBlockingWarning:       true,
			expected:                 "The upgrade is retargeted to 4.7.3 from the existing minor level upgrade from 4.6.3 to 4.7.2.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clusterVersion := &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec:       configv1.ClusterVersionSpec{},
				Status: configv1.ClusterVersionStatus{
					History: []configv1.UpdateHistory{},
					Desired: configv1.Release{Version: tc.versionPartiallyUpgraded},
				},
			}
			if len(tc.currVersion) > 0 {
				clusterVersion.Status.History = append(clusterVersion.Status.History, configv1.UpdateHistory{Version: tc.currVersion, State: configv1.CompletedUpdate})
			}
			if tc.versionPartiallyUpgraded != "" {
				clusterVersion.Status.History = append(clusterVersion.Status.History, configv1.UpdateHistory{Version: tc.versionPartiallyUpgraded, State: configv1.PartialUpdate})
			}
			if tc.upgradeable != nil {
				clusterVersion.Status.Conditions = append(clusterVersion.Status.Conditions, configv1.ClusterOperatorStatusCondition{
					Type:    configv1.OperatorUpgradeable,
					Status:  *tc.upgradeable,
					Message: fmt.Sprintf("set to %v", *tc.upgradeable),
				})
			}

			if tc.versionPartiallyUpgraded != "" {
				clusterVersion.Status.Conditions = append(clusterVersion.Status.Conditions, configv1.ClusterOperatorStatusCondition{
					Type:    UpgradeInProgress,
					Status:  configv1.ConditionTrue,
					Message: "some-message",
					Reason:  "some-reason",
				})
			} else {
				clusterVersion.Status.Conditions = append(clusterVersion.Status.Conditions, configv1.ClusterOperatorStatusCondition{
					Type:    UpgradeInProgress,
					Status:  configv1.ConditionFalse,
					Message: "message-bar",
					Reason:  "reason-bar",
				})
			}
			cvLister := fakeClusterVersionLister(t, clusterVersion)
			instance := NewUpgradeable(cvLister)

			err := instance.Run(ctx, precondition.ReleaseContext{
				DesiredVersion: tc.desiredVersion,
			})
			if tc.NonBlockingWarning {
				pError, ok := err.(*precondition.Error)
				if !ok {
					t.Errorf("Failed to convert to err: %v", err)
				} else if pError.NonBlockingWarning != true {
					t.Error("NonBlockingWarning should be true")
				}
			}
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
