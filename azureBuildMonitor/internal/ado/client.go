// Package ado wraps the official Azure DevOps Go SDK down to just the two
// operations this app needs: a bulk "top N builds per definition" query
// for the hourly full refresh, and a single "get build by ID" lookup used
// to enrich real-time webhook events (whose payload doesn't reliably
// carry the definition ID or branch name -- see the webhook package).
package ado

import (
	"context"
	"fmt"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
)

type Client struct {
	build       build.Client
	projectName string
}

func NewClient(ctx context.Context, orgURL, pat, projectName string) (*Client, error) {
	conn := azuredevops.NewPatConnection(orgURL, pat)
	buildClient, err := build.NewClient(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("creating Azure DevOps build client: %w", err)
	}
	return &Client{build: buildClient, projectName: projectName}, nil
}

// LatestBuilds fetches, in one call, the most recent maxPerDefinition
// builds for each of the given definition IDs.
func (c *Client) LatestBuilds(ctx context.Context, definitionIDs []int, maxPerDefinition int) (map[int][]store.BuildInfo, map[int]string, error) {
	order := build.BuildQueryOrderValues.QueueTimeDescending
	resp, err := c.build.GetBuilds(ctx, build.GetBuildsArgs{
		Project:                &c.projectName,
		Definitions:            &definitionIDs,
		MaxBuildsPerDefinition: &maxPerDefinition,
		QueryOrder:             &order,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("GetBuilds: %w", err)
	}

	byDefinition := make(map[int][]store.BuildInfo)
	names := make(map[int]string)
	for _, b := range resp.Value {
		info, defID, defName := fromSDKBuild(b)
		if defID == 0 {
			continue
		}
		byDefinition[defID] = append(byDefinition[defID], info)
		if defName != "" {
			names[defID] = defName
		}
	}
	return byDefinition, names, nil
}

// GetBuild fetches full details for a single build/run by ID -- used to
// enrich webhook events, which don't reliably include the definition ID
// or branch name.
func (c *Client) GetBuild(ctx context.Context, buildID int) (info store.BuildInfo, definitionID int, definitionName string, err error) {
	b, getErr := c.build.GetBuild(ctx, build.GetBuildArgs{
		Project: &c.projectName,
		BuildId: &buildID,
	})
	if getErr != nil {
		return store.BuildInfo{}, 0, "", fmt.Errorf("GetBuild(%d): %w", buildID, getErr)
	}
	info, definitionID, definitionName = fromSDKBuild(*b)
	return info, definitionID, definitionName, nil
}

func fromSDKBuild(b build.Build) (info store.BuildInfo, definitionID int, definitionName string) {
	if b.Id != nil {
		info.ID = *b.Id
	}
	if b.BuildNumber != nil {
		info.Number = *b.BuildNumber
	}
	if b.SourceBranch != nil {
		info.SourceBranch = *b.SourceBranch
	}
	if b.QueueTime != nil {
		info.QueueTime = b.QueueTime.Time
	}
	if b.StartTime != nil {
		info.StartTime = b.StartTime.Time
	}
	if b.FinishTime != nil {
		info.FinishTime = b.FinishTime.Time
	}

	var state, result string
	if b.Status != nil {
		state = string(*b.Status)
	}
	if b.Result != nil {
		result = string(*b.Result)
	}
	info.Status = store.NormalizeStatus(state, result)

	if b.Definition != nil {
		if b.Definition.Id != nil {
			definitionID = *b.Definition.Id
		}
		if b.Definition.Name != nil {
			definitionName = *b.Definition.Name
		}
	}
	return info, definitionID, definitionName
}
