package log

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	crypto "github.com/libp2p/go-libp2p-crypto"
	"github.com/qri-io/dataset"
	"github.com/qri-io/qfs"
)

func Example() {
	// logbooks are encrypted at rest, we need a private key to interact with
	// them, including to create a new logbook. This is a dummy Private Key
	// you should never, ever use in real life. demo only folks.
	testPk := `CAASpgkwggSiAgEAAoIBAQC/7Q7fILQ8hc9g07a4HAiDKE4FahzL2eO8OlB1K99Ad4L1zc2dCg+gDVuGwdbOC29IngMA7O3UXijycckOSChgFyW3PafXoBF8Zg9MRBDIBo0lXRhW4TrVytm4Etzp4pQMyTeRYyWR8e2hGXeHArXM1R/A/SjzZUbjJYHhgvEE4OZy7WpcYcW6K3qqBGOU5GDMPuCcJWac2NgXzw6JeNsZuTimfVCJHupqG/dLPMnBOypR22dO7yJIaQ3d0PFLxiDG84X9YupF914RzJlopfdcuipI+6gFAgBw3vi6gbECEzcohjKf/4nqBOEvCDD6SXfl5F/MxoHurbGBYB2CJp+FAgMBAAECggEAaVOxe6Y5A5XzrxHBDtzjlwcBels3nm/fWScvjH4dMQXlavwcwPgKhy2NczDhr4X69oEw6Msd4hQiqJrlWd8juUg6vIsrl1wS/JAOCS65fuyJfV3Pw64rWbTPMwO3FOvxj+rFghZFQgjg/i45uHA2UUkM+h504M5Nzs6Arr/rgV7uPGR5e5OBw3lfiS9ZaA7QZiOq7sMy1L0qD49YO1ojqWu3b7UaMaBQx1Dty7b5IVOSYG+Y3U/dLjhTj4Hg1VtCHWRm3nMOE9cVpMJRhRzKhkq6gnZmni8obz2BBDF02X34oQLcHC/Wn8F3E8RiBjZDI66g+iZeCCUXvYz0vxWAQQKBgQDEJu6flyHPvyBPAC4EOxZAw0zh6SF/r8VgjbKO3n/8d+kZJeVmYnbsLodIEEyXQnr35o2CLqhCvR2kstsRSfRz79nMIt6aPWuwYkXNHQGE8rnCxxyJmxV4S63GczLk7SIn4KmqPlCI08AU0TXJS3zwh7O6e6kBljjPt1mnMgvr3QKBgQD6fAkdI0FRZSXwzygx4uSg47Co6X6ESZ9FDf6ph63lvSK5/eue/ugX6p/olMYq5CHXbLpgM4EJYdRfrH6pwqtBwUJhlh1xI6C48nonnw+oh8YPlFCDLxNG4tq6JVo071qH6CFXCIank3ThZeW5a3ZSe5pBZ8h4bUZ9H8pJL4C7yQKBgFb8SN/+/qCJSoOeOcnohhLMSSD56MAeK7KIxAF1jF5isr1TP+rqiYBtldKQX9bIRY3/8QslM7r88NNj+aAuIrjzSausXvkZedMrkXbHgS/7EAPflrkzTA8fyH10AsLgoj/68mKr5bz34nuY13hgAJUOKNbvFeC9RI5g6eIqYH0FAoGAVqFTXZp12rrK1nAvDKHWRLa6wJCQyxvTU8S1UNi2EgDJ492oAgNTLgJdb8kUiH0CH0lhZCgr9py5IKW94OSM6l72oF2UrS6PRafHC7D9b2IV5Al9lwFO/3MyBrMocapeeyaTcVBnkclz4Qim3OwHrhtFjF1ifhP9DwVRpuIg+dECgYANwlHxLe//tr6BM31PUUrOxP5Y/cj+ydxqM/z6papZFkK6Mvi/vMQQNQkh95GH9zqyC5Z/yLxur4ry1eNYty/9FnuZRAkEmlUSZ/DobhU0Pmj8Hep6JsTuMutref6vCk2n02jc9qYmJuD7iXkdXDSawbEG6f5C4MUkJ38z1t1OjA==`
	data, err := base64.StdEncoding.DecodeString(testPk)
	if err != nil {
		panic(err)
	}
	pk, err := crypto.UnmarshalPrivateKey(data)
	if err != nil {
		panic(fmt.Errorf("error unmarshaling private key: %s", err.Error()))
	}

	// logbook relies on a qfs.Filesystem for read & write. create an in-memory
	// filesystem we can play with
	fs := qfs.NewMemFS(nil)

	// Create a new LogBook, passing in:
	//  * the author private key to encrypt & decrypt the logbook
	//  * author's current username
	//  * a qfs.Filesystem for reading & writing the logbook
	//  * a base path on the filesystem to read & write the logbook to
	// Initializing a logbook ensures the author has an user opset that matches
	// their current state. It will error if a stored book can't be decrypted
	book, err := NewLogbook(pk, "b5", fs, "/mem/logset")
	if err != nil {
		panic(err) // real programs don't panic
	}

	// create a name to store dataset versions in. NameInit will create a new
	// log under the logbook author's namespace with the given name, and an opset
	// that tracks operations by this author within that new namespace.
	// The entire logbook is persisted to the filestore after each operation
	if err := book.NameInit("world_bank_population"); err != nil {
		panic(err)
	}

	// pretend we've just created a dataset, these are the only fields the log
	// will care about
	ds := &dataset.Dataset{
		Commit: &dataset.Commit{
			Timestamp: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC),
			Title:     "initial commit",
		},
		Path:         "QmHashOfVersion1",
		PreviousPath: "",
		// TODO (b5) - at some point we may want to log parent versions as well,
		// need to model those properly first.
	}

	// create a log record of the version of a dataset. In practice this'll be
	// part of the overall save routine
	if err := book.VersionSave("me/world_bank_poulation", ds); err != nil {
		panic(err)
	}

	// sometime later, we create another version
	ds2 := &dataset.Dataset{
		Commit: &dataset.Commit{
			Timestamp: time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC),
			Title:     "added body data",
		},
		Structure: &dataset.Structure{
			Length: 100,
		},
		Path:         "QmHashOfVersion2",
		PreviousPath: "",
	}

	// once again, write to the log
	if err := book.VersionSave("me/world_bank_population", ds2); err != nil {
		panic(err)
	}

	// pretend we just published both saved versions of the dataset to the
	// registry we log that here. Providing a revisions arg of 2 means we've
	// published two consecutive revisions from head: the latest version, and the
	// one before it. "registry.qri.cloud" indicates we published to a single
	// destination with that name.
	if err := book.Publish("me/world_bank_population", 2, "registry.qri.cloud"); err != nil {
		panic(err)
	}

	// pretend the user just deleted a dataset version, well, we need to log it!
	// VersionDelete accepts an argument of number of versions back from HEAD
	// more complex deletes that remove pieces of history may require either
	// composing multiple log operations
	book.VersionDelete("me/world_bank_population", 1)

	// create another version
	ds3 := &dataset.Dataset{
		Commit: &dataset.Commit{
			Timestamp: time.Date(2000, time.January, 3, 0, 0, 0, 0, time.UTC),
			Title:     "added meta info",
		},
		Structure: &dataset.Structure{
			Length: 100,
		},
		Path: "QmHashOfVersion3",
		// note that we're referring to version 1 here. version 2 no longer exists
		// this is happening outside of the log, but the log should reflect
		// contiguous history
		PreviousPath: "QmHashOfVersion1",
	}

	// once again, write to the log
	if err := book.VersionSave("me/world_bank_population", ds3); err != nil {
		panic(err)
	}

	// now for the fun bit. When we ask for the state of the log, it will
	// play our opsets forward and get us the current state of tne log
	// we can also get the state of a log from the book:
	state := book.State("me/world_bank_population")

	fmt.Println(state)
	// Output:
	// TODO (b5) - figure out what's actually output here
}

type testRunner struct {
	ctx      context.Context
	Username string
}

func newTestRunner(t *testing.T) (tr *testRunner, cleanup func()) {
	return &testRunner{}, func() {}
}

// func TestLogInit(t *testing.T) {
// }

func TestLogState(t *testing.T) {
	// logA := Log{}
	// logA, _ = PutDatasetCommit(logA, "PeerA", "CommitA", "", "created dataset")
	// logA, _ = PutSuggestionUpdate(logA, "PeerA", "SuggestA", "", "Hey I'm a comment")

	// logB := logA.Copy()
	// logB, _ = PutDatasetCommit(logB, "PeerB", "CommitB", "CommitA", "added the stuff")
	// logB, _ = PutSuggestionUpdate(logB, "PeerB", "SuggestD", "", "this so cool")

	// logA, _ = PutSuggestionUpdate(logA, "PeerA", "SuggestB", "", "Another Comment")
	// logA, _ = PutSuggestionUpdate(logA, "PeerA", "SuggestC", "SuggestA", "updated comment")
	// logA, _ = PutSuggestionDelete(logA, "PeerA", "SuggestC")

	// logA = logA.Put(logB.Ops[len(logB.Ops)-1])

	// logA, _ = PutDatasetCommit(logA, "PeerA", "CommitC", "CommitA", "an edit")
	// logB = logB.Put(logA.Ops[len(logA.Ops)-1])

	// s := logA.State()
	// data, _ := json.MarshalIndent(s, "", "  ")
	// fmt.Println(string(data))
}
