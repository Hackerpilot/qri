package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/qri-io/qri/config"
	"github.com/qri-io/qri/repo/profile"
	testrepo "github.com/qri-io/qri/repo/test"
)

func TestProfileRequestsGet(t *testing.T) {
	cases := []struct {
		in  bool
		res *profile.Profile
		err string
	}{
		// {true, nil, ""},
		// {false, nil, ""},
	}

	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Errorf("error allocating test repo: %s", err.Error())
		return
	}

	req := NewProfileRequests(mr, nil)
	for i, c := range cases {
		got := &config.ProfilePod{}
		err := req.GetProfile(&c.in, got)

		if !(err == nil && c.err == "" || err != nil && err.Error() == c.err) {
			t.Errorf("case %d error mismatch: expected: %s, got: %s", i, c.err, err)
			continue
		}
	}
}

func TestProfileRequestsSave(t *testing.T) {
	prev := SaveConfig
	SaveConfig = func() error {
		return nil
	}
	defer func() { SaveConfig = prev }()

	cases := []struct {
		p   *config.ProfilePod
		res *config.ProfilePod
		err string
	}{
		{nil, nil, "profile required for update"},
		{&config.ProfilePod{}, nil, ""},
		// TODO - moar tests
	}

	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Errorf("error allocating test repo: %s", err.Error())
		return
	}

	req := NewProfileRequests(mr, nil)
	for i, c := range cases {
		got := &config.ProfilePod{}
		err := req.SaveProfile(c.p, got)

		if !(err == nil && c.err == "" || err != nil && err.Error() == c.err) {
			t.Errorf("case %d error mismatch: expected: %s, got: %s", i, c.err, err)
			continue
		}
	}
}

func TestSaveProfile(t *testing.T) {
	// Mock data for the global Config's Profile, used to create new profile.
	// TODO: Remove the randomly built Profile that config.DefaultProfile creates.
	mockID := "QmWu3MKx2B1xxphkSNWxw8TYt41HnXD8R85Kt2UzKzpGH9"
	mockTime := time.Unix(1234567890, 0)
	Config.Profile.ID = mockID
	Config.Profile.Created = mockTime
	Config.Profile.Updated = mockTime
	Config.Profile.Peername = "test_mock_peer_name"

	// SaveConfig func replacement, so that it copies the global Config here.
	var savedConf config.Config
	prevSaver := SaveConfig
	SaveConfig = func() error {
		savedConf = *Config
		return nil
	}
	defer func() { SaveConfig = prevSaver }()

	// CodingProfile filled with test data.
	pro := config.ProfilePod{}
	pro.Name = "test_name"
	pro.Email = "test_email@example.com"
	pro.Description = "This is only a test profile"
	pro.HomeURL = "http://example.com"
	pro.Color = "default"
	pro.Twitter = "test_twitter"

	// Save the CodingProfile.
	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Errorf("error allocating test repo: %s", err.Error())
		return
	}
	req := NewProfileRequests(mr, nil)
	got := config.ProfilePod{}
	err = req.SaveProfile(&pro, &got)
	if err != nil {
		log.Fatal(err)
	}

	// Saving adds a private key. Verify that it used to not exist, then copy the key.
	if got.PrivKey != "" {
		log.Errorf("Returned Profile should not have private key: %v", got.PrivKey)
	}
	got.PrivKey = savedConf.Profile.PrivKey

	// Verify that the saved config matches the returned config (after private key is copied).
	if !reflect.DeepEqual(*savedConf.Profile, got) {
		log.Errorf("Saved Profile does not match returned Profile: %v <> %v",
			*savedConf.Profile, got)
	}

	// Validate that the returned Profile has all the proper individual fields.
	nullTime := "0001-01-01 00:00:00 +0000 UTC"
	if pro.ID != "" {
		log.Errorf("Profile should not have ID, has %v", pro.ID)
	}
	if got.ID != mockID {
		log.Errorf("Got ID %v", got.ID)
	}
	if pro.Created.String() != nullTime {
		log.Errorf("Profile should not have Created, has %v", pro.Created)
	}
	if got.Created != mockTime {
		log.Errorf("Got Created %v", got.Created)
	}
	if pro.Updated.String() != nullTime {
		log.Errorf("Profile should not have Updated, has %v", pro.Updated)
	}
	if got.Updated != mockTime {
		log.Errorf("Got Updated %v", got.Updated)
	}
	if got.Type != "peer" {
		log.Errorf("Got Type %v", got.Type)
	}
	if got.Peername != "test_mock_peer_name" {
		log.Errorf("Got Peername %v", got.Peername)
	}
	if got.Name != "test_name" {
		log.Errorf("Got Name %v", got.Name)
	}
	if got.Email != "test_email@example.com" {
		log.Errorf("Got Email %v", got.Email)
	}
	if got.Description != "This is only a test profile" {
		log.Errorf("Got Description %v", got.Description)
	}
	if got.HomeURL != "http://example.com" {
		log.Errorf("Got Type %v", got.HomeURL)
	}
	if got.Color != "default" {
		log.Errorf("Got Type %v", got.Color)
	}
	if got.Twitter != "test_twitter" {
		log.Errorf("Got Type %v", got.Twitter)
	}
}

func TestProfileRequestsSetProfilePhoto(t *testing.T) {
	prev := SaveConfig
	SaveConfig = func() error {
		return nil
	}
	defer func() { SaveConfig = prev }()

	cases := []struct {
		infile  string
		respath datastore.Key
		err     string
	}{
		{"", datastore.NewKey(""), "file is required"},
		{"testdata/ink_big_photo.jpg", datastore.NewKey(""), "file size too large. max size is 250kb"},
		{"testdata/q_bang.svg", datastore.NewKey(""), "invalid file format. only .jpg images allowed"},
		{"testdata/rico_400x400.jpg", datastore.NewKey("/map/QmRdexT18WuAKVX3vPusqmJTWLeNSeJgjmMbaF5QLGHna1"), ""},
	}

	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Errorf("error allocating test repo: %s", err.Error())
		return
	}

	req := NewProfileRequests(mr, nil)
	for i, c := range cases {
		p := &FileParams{}
		if c.infile != "" {
			p.Filename = filepath.Base(c.infile)
			r, err := os.Open(c.infile)
			if err != nil {
				t.Errorf("case %d error opening test file %s: %s ", i, c.infile, err.Error())
				continue
			}
			p.Data = r
		}

		res := &config.ProfilePod{}
		err := req.SetProfilePhoto(p, res)
		if !(err == nil && c.err == "" || err != nil && err.Error() == c.err) {
			t.Errorf("case %d error mismatch. expected: %s, got: %s", i, c.err, err.Error())
			continue
		}

		if !c.respath.Equal(datastore.NewKey(res.Photo)) {
			t.Errorf("case %d profile hash mismatch. expected: %s, got: %s", i, c.respath.String(), res.Photo)
			continue
		}
	}
}

func TestProfileRequestsSetPosterPhoto(t *testing.T) {
	prev := SaveConfig
	SaveConfig = func() error {
		return nil
	}
	defer func() { SaveConfig = prev }()

	cases := []struct {
		infile  string
		respath datastore.Key
		err     string
	}{
		{"", datastore.NewKey(""), "file is required"},
		{"testdata/ink_big_photo.jpg", datastore.NewKey(""), "file size too large. max size is 250kb"},
		{"testdata/q_bang.svg", datastore.NewKey(""), "invalid file format. only .jpg images allowed"},
		{"testdata/rico_poster_1500x500.jpg", datastore.NewKey("/map/QmdJgfxj4rocm88PLeEididS7V2cc9nQosA46RpvAnWvDL"), ""},
	}

	mr, err := testrepo.NewTestRepo()
	if err != nil {
		t.Errorf("error allocating test repo: %s", err.Error())
		return
	}

	req := NewProfileRequests(mr, nil)
	for i, c := range cases {
		p := &FileParams{}
		if c.infile != "" {
			p.Filename = filepath.Base(c.infile)
			r, err := os.Open(c.infile)
			if err != nil {
				t.Errorf("case %d error opening test file %s: %s ", i, c.infile, err.Error())
				continue
			}
			p.Data = r
		}

		res := &config.ProfilePod{}
		err := req.SetProfilePhoto(p, res)
		if !(err == nil && c.err == "" || err != nil && err.Error() == c.err) {
			t.Errorf("case %d error mismatch. expected: %s, got: %s", i, c.err, err.Error())
			continue
		}

		if !c.respath.Equal(datastore.NewKey(res.Photo)) {
			t.Errorf("case %d profile hash mismatch. expected: %s, got: %s", i, c.respath.String(), res.Photo)
			continue
		}
	}
}
