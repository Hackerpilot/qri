package actions

import (
	"context"
	"testing"

	"github.com/qri-io/qri/repo"
)

func TestListDatasets(t *testing.T) {
	ctx := context.Background()
	node := newTestNode(t)
	addCitiesDataset(t, node.Repo)

	res, err := ListDatasets(ctx, node, &repo.DatasetRef{Peername: "me"}, "", 1, 0, false, false, false)
	if err != nil {
		t.Error(err.Error())
	}

	if len(res) != 1 {
		t.Error("expected one dataset response")
	}
	if res[0].Dataset.NumVersions != 0 {
		t.Error("expected no versions were requested")
	}
}

func TestListDatasetsNotFound(t *testing.T) {
	ctx := context.Background()
	node := newTestNode(t)
	addCitiesDataset(t, node.Repo)

	_, err := ListDatasets(ctx, node, &repo.DatasetRef{Peername: "not_found"}, "", 1, 0, false, false, false)
	if err == nil {
		t.Error("expected to get error")
	}
	expect := "profile not found: \"not_found\""
	if expect != err.Error() {
		t.Errorf("expected error \"%s\", got \"%s\"", expect, err.Error())
	}
}

func TestListDatasetsWithVersions(t *testing.T) {
	ctx := context.Background()
	node := newTestNode(t)
	addCitiesDataset(t, node.Repo)

	res, err := ListDatasets(ctx, node, &repo.DatasetRef{Peername: "me"}, "", 1, 0, false, false, true)
	if err != nil {
		t.Error(err.Error())
	}

	if len(res) != 1 {
		t.Error("expected one dataset response")
	}
	if res[0].Dataset.NumVersions != 1 {
		t.Error("expected one version")
	}
}
