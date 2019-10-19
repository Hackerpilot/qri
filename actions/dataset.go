package actions

import (
	"context"
	"fmt"
	"io"

	"github.com/qri-io/dataset"
	"github.com/qri-io/dataset/dsfs"
	"github.com/qri-io/ioes"
	"github.com/qri-io/qfs"
	"github.com/qri-io/qfs/cafs"
	"github.com/qri-io/qri/base"
	"github.com/qri-io/qri/p2p"
	"github.com/qri-io/qri/remote"
	"github.com/qri-io/qri/repo"
	"github.com/qri-io/qri/repo/profile"
	"github.com/qri-io/qri/startf"
)

// SaveDatasetSwitches provides toggleable flags to SaveDataset that control
// save behaviour
type SaveDatasetSwitches struct {
	Replace             bool
	DryRun              bool
	Pin                 bool
	ConvertFormatToPrev bool
	Force               bool
	ShouldRender        bool
}

// SaveDataset initializes a dataset from a dataset pointer and data file
func SaveDataset(ctx context.Context, r repo.Repo, str ioes.IOStreams, changes *dataset.Dataset, secrets map[string]string, scriptOut io.Writer, sw SaveDatasetSwitches) (ref repo.DatasetRef, err error) {
	var (
		prevPath string
		pro      *profile.Profile
	)

	prev, mutable, prevPath, err := base.PrepareDatasetSave(ctx, r, changes.Peername, changes.Name)
	if err != nil {
		return
	}

	if pro, err = r.Profile(); err != nil {
		return
	}

	if sw.DryRun {
		str.PrintErr("🏃🏽‍♀️ dry run\n")

		// dry-runs store to an in-memory repo
		r, err = repo.NewMemRepo(pro, cafs.NewMapstore(), r.Filesystem(), profile.NewMemStore())
		if err != nil {
			return
		}
	}

	if changes.Transform != nil {
		// create a check func from a record of all the parts that the datasetPod is changing,
		// the startf package will use this function to ensure the same components aren't modified
		mutateCheck := startf.MutatedComponentsFunc(changes)

		opts := []func(*startf.ExecOpts){
			startf.AddQriRepo(r),
			startf.AddMutateFieldCheck(mutateCheck),
			startf.SetOutWriter(scriptOut),
			startf.SetSecrets(secrets),
		}

		if err = startf.ExecScript(ctx, changes, prev, opts...); err != nil {
			return
		}

		str.PrintErr("✅ transform complete\n")
	}

	if prevPath == "" && changes.BodyFile() == nil && changes.Structure == nil {
		err = fmt.Errorf("creating a new dataset requires a structure or a body")
		return
	}

	if changes.BodyFile() != nil && prev.Structure != nil && changes.Structure != nil && prev.Structure.Format != changes.Structure.Format {
		if sw.ConvertFormatToPrev {
			var f qfs.File
			f, err = base.ConvertBodyFormat(changes.BodyFile(), changes.Structure, prev.Structure)
			if err != nil {
				return
			}
			// Set the new format on the change structure.
			changes.Structure.Format = prev.Structure.Format
			changes.SetBodyFile(f)
		} else {
			err = fmt.Errorf("Refusing to change structure from %s to %s",
				prev.Structure.Format, changes.Structure.Format)
			return
		}
	}

	if !sw.Replace {
		// Treat the changes as a set of patches applied to the previous dataset
		mutable.Assign(changes)
		changes = mutable
	}

	// infer missing values
	if err = base.InferValues(pro, changes); err != nil {
		return
	}

	// add a default viz if one is needed
	if sw.ShouldRender {
		base.MaybeAddDefaultViz(changes)
	}

	// let's make history, if it exists
	changes.PreviousPath = prevPath

	return base.CreateDataset(ctx, r, str, changes, prev, sw.DryRun, sw.Pin, sw.Force, sw.ShouldRender)
}

// UpdateRemoteDataset brings a reference to the latest version, syncing to the
// latest history it can find over p2p & via any configured registry
func UpdateRemoteDataset(ctx context.Context, node *p2p.QriNode, ref *repo.DatasetRef, pin bool) (res repo.DatasetRef, err error) {
	return res, fmt.Errorf("remote updating is currently disabled")
}

// AddDataset fetches & pins a dataset to the store, adding it to the list of stored refs
func AddDataset(ctx context.Context, node *p2p.QriNode, rc *remote.Client, remoteAddr string, ref *repo.DatasetRef) (err error) {
	log.Debugf("add dataset %s. remoteAddr: %s", ref.String(), remoteAddr)
	if !ref.Complete() {
		// TODO (ramfox): we should check to see if the dataset already exists locally
		// unfortunately, because of the nature of the ipfs filesystem commands, we don't
		// know if files we fetch are local only or possibly coming from the network.
		// instead, for now, let's just always try to add
		if _, err := ResolveDatasetRef(ctx, node, rc, remoteAddr, ref); err != nil {
			return err
		}
	}

	type addResponse struct {
		Ref   *repo.DatasetRef
		Error error
	}

	fetchCtx, cancelFetch := context.WithCancel(ctx)
	defer cancelFetch()
	responses := make(chan addResponse)
	tasks := 0

	if rc != nil && remoteAddr != "" {
		tasks++

		refCopy := &repo.DatasetRef{
			Peername:  ref.Peername,
			ProfileID: ref.ProfileID,
			Name:      ref.Name,
			Path:      ref.Path,
		}

		go func(ref *repo.DatasetRef) {
			res := addResponse{Ref: ref}

			// always send on responses channel
			defer func() {
				responses <- res
			}()

			if err := rc.PullDataset(fetchCtx, ref, remoteAddr); err != nil {
				res.Error = err
				return
			}
			node.LocalStreams.PrintErr("🗼 fetched from registry\n")
			if pinner, ok := node.Repo.Store().(cafs.Pinner); ok {
				err := pinner.Pin(fetchCtx, ref.Path, true)
				res.Error = err
			}
		}(refCopy)
	}

	if node.Online {
		tasks++
		go func() {
			err := base.FetchDataset(fetchCtx, node.Repo, ref, true, true)
			responses <- addResponse{
				Ref:   ref,
				Error: err,
			}
		}()
	}

	if tasks == 0 {
		return fmt.Errorf("no registry configured and node is not online")
	}

	success := false
	for i := 0; i < tasks; i++ {
		res := <-responses
		err = res.Error
		if err == nil {
			cancelFetch()
			success = true
			*ref = *res.Ref
			break
		}
	}

	if !success {
		return fmt.Errorf("add failed: %s", err.Error())
	}

	prevRef, err := node.Repo.GetRef(repo.DatasetRef{Peername: ref.Peername, Name: ref.Name})
	if err != nil && err == repo.ErrNotFound {
		if err = node.Repo.PutRef(*ref); err != nil {
			log.Debug(err.Error())
			return fmt.Errorf("error putting dataset in repo: %s", err.Error())
		}
		return nil
	}
	if err != nil {
		return err
	}

	prevRef.Dataset, err = dsfs.LoadDataset(ctx, node.Repo.Store(), prevRef.Path)
	if err != nil {
		log.Debug(err.Error())
		return fmt.Errorf("error loading repo dataset: %s", prevRef.Path)
	}

	ref.Dataset, err = dsfs.LoadDataset(ctx, node.Repo.Store(), ref.Path)
	if err != nil {
		log.Debug(err.Error())
		return fmt.Errorf("error loading added dataset: %s", ref.Path)
	}

	return base.ReplaceRefIfMoreRecent(node.Repo, &prevRef, ref)
}
