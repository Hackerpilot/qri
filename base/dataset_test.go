package base

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/qri-io/dataset/dstest"
	"github.com/qri-io/qfs/cafs"
	"github.com/qri-io/qri/base/dsfs"
	"github.com/qri-io/qri/repo"
	reporef "github.com/qri-io/qri/repo/ref"
	repotest "github.com/qri-io/qri/repo/test"
)

func TestListDatasets(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	ref := addCitiesDataset(t, r)

	// Limit to one
	res, err := ListDatasets(ctx, r, "", 1, 0, false, false, false)
	if err != nil {
		t.Error(err.Error())
	}
	if len(res) != 1 {
		t.Error("expected one dataset response")
	}

	// Limit to published datasets
	res, err = ListDatasets(ctx, r, "", 1, 0, false, true, false)
	if err != nil {
		t.Error(err.Error())
	}

	if len(res) != 0 {
		t.Error("expected no published datasets")
	}

	if err := SetPublishStatus(r, &ref, true); err != nil {
		t.Fatal(err)
	}

	// Limit to published datasets, after publishing cities
	res, err = ListDatasets(ctx, r, "", 1, 0, false, true, false)
	if err != nil {
		t.Error(err.Error())
	}

	if len(res) != 1 {
		t.Error("expected one published dataset response")
	}

	// Limit to datasets with "city" in their name
	res, err = ListDatasets(ctx, r, "city", 1, 0, false, false, false)
	if err != nil {
		t.Error(err.Error())
	}
	if len(res) != 0 {
		t.Error("expected no datasets with \"city\" in their name")
	}

	// Limit to datasets with "cit" in their name
	res, err = ListDatasets(ctx, r, "cit", 1, 0, false, false, false)
	if err != nil {
		t.Error(err.Error())
	}
	if len(res) != 1 {
		t.Error("expected one dataset with \"cit\" in their name")
	}
}

func TestFetchDataset(t *testing.T) {
	ctx := context.Background()
	r1 := newTestRepo(t)
	r2 := newTestRepo(t)
	ref := addCitiesDataset(t, r2)

	// Connect in memory Mapstore's behind the scene to simulate IPFS-like behavior.
	r1.Store().(*cafs.MapStore).AddConnection(r2.Store().(*cafs.MapStore))

	if err := FetchDataset(ctx, r1, &reporef.DatasetRef{Peername: "foo", Name: "bar"}, true, true); err == nil {
		t.Error("expected add of invalid ref to error")
	}

	if err := FetchDataset(ctx, r1, &ref, true, true); err != nil {
		t.Error(err.Error())
	}
}

func TestDatasetPinning(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	ref := addCitiesDataset(t, r)

	if err := PinDataset(ctx, r, ref); err != nil {
		if err == repo.ErrNotPinner {
			t.Log("repo store doesn't support pinning")
		} else {
			t.Error(err.Error())
			return
		}
	}

	tc, err := dstest.NewTestCaseFromDir(repotest.TestdataPath("counter"))
	if err != nil {
		t.Error(err.Error())
		return
	}

	ref2, err := CreateDataset(ctx, r, devNull, tc.Input, nil, false, false, false, true)
	if err != nil {
		t.Error(err.Error())
		return
	}

	if err := PinDataset(ctx, r, ref2); err != nil && err != repo.ErrNotPinner {
		// TODO (b5) - not sure what's going on here
		t.Log(err.Error())
		return
	}

	if err := UnpinDataset(ctx, r, ref); err != nil && err != repo.ErrNotPinner {
		t.Error(err.Error())
		return
	}

	if err := UnpinDataset(ctx, r, ref2); err != nil && err != repo.ErrNotPinner {
		t.Error(err.Error())
		return
	}
}

func TestRawDatasetRefs(t *testing.T) {
	// to keep hashes consistent, artificially specify the timestamp by overriding
	// the dsfs.Timestamp func
	prev := dsfs.Timestamp
	defer func() { dsfs.Timestamp = prev }()
	minute := 0
	dsfs.Timestamp = func() time.Time {
		minute++
		return time.Date(2001, 01, 01, 01, minute, 01, 01, time.UTC)
	}

	ctx := context.Background()
	r := newTestRepo(t)
	addCitiesDataset(t, r)

	actual, err := RawDatasetRefs(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	expect := `0 Peername:  peer
  ProfileID: 9tmwSYB7dPRUXaEwJRNgzb6NbwPYNXrYyeahyHPAUqrTYd3Z6bVS9z1mCDsRmvb
  Name:      cities
  Path:      /map/QmbU34XVYPGeEGjJ93rBm4Nac2g4hBYFouDnu9p9psccDB
  FSIPath:   
  Published: false
`
	if diff := cmp.Diff(expect, actual); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}
