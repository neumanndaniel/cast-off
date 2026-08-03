// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package controller

import (
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

func TestDeployedReleaseInfoSelectsLatestRevision(t *testing.T) {
	releases := []*release.Release{
		{
			Name:    "cilium",
			Version: 3,
			Info:    &release.Info{Status: release.StatusDeployed},
			Chart: &chart.Chart{Metadata: &chart.Metadata{
				AppVersion: "1.16.6",
			}},
		},
		{
			Name:    "cilium",
			Version: 5,
			Info:    &release.Info{Status: release.StatusDeployed},
			Chart: &chart.Chart{Metadata: &chart.Metadata{
				AppVersion: "1.17.0",
			}},
		},
	}

	version, revision, err := deployedReleaseInfo(releases, "cilium")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "1.17.0" {
		t.Fatalf("unexpected version: %s", version)
	}
	if revision != 5 {
		t.Fatalf("unexpected revision: %d", revision)
	}
}

func TestDeployedReleaseInfoErrorsOnMissingMetadata(t *testing.T) {
	releases := []*release.Release{
		{
			Name:    "cilium",
			Version: 1,
			Info:    &release.Info{Status: release.StatusDeployed},
			Chart:   nil,
		},
	}

	_, _, err := deployedReleaseInfo(releases, "cilium")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no chart metadata") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeployedReleaseInfoErrorsWhenNoRelease(t *testing.T) {
	_, _, err := deployedReleaseInfo(nil, "cilium")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
