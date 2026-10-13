package ota

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog"
)

// A platform installer receives the verified download before the reboot, and
// a release server without a system component does not fail the update.
func TestAppInstallerRunsBeforeReboot(t *testing.T) {
	payload := []byte("application package")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			_ = json.NewEncoder(w).Encode(UpdateMetadata{
				AppVersion: "1.0.3+vim4.20261013.31",
				AppURL:     "http://" + r.Host + "/app.deb",
				AppHash:    hex.EncodeToString(sum[:]),
			})
		case "/app.deb":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var installed []byte
	var rebooted bool
	logger := zerolog.Nop()
	path := filepath.Join(t.TempDir(), "update.deb")
	state := NewState(Options{
		Logger:             &logger,
		SkipConfirmSystem:  true,
		ReleaseAPIEndpoint: server.URL + "/releases",
		GetHTTPClient:      func() HttpClient { return server.Client() },
		GetLocalVersion: func() (*semver.Version, *semver.Version, error) {
			return semver.MustParse("0.0.0"), semver.MustParse("1.0.3+vim4.20261013.30"), nil
		},
		HwReboot: func(bool, *PostRebootAction, time.Duration) error {
			if installed == nil {
				t.Error("rebooted before installation")
			}
			rebooted = true
			return nil
		},
		OnStateUpdate:    func(*RPCState) {},
		OnProgressUpdate: func(float32) {},
		AppUpdatePath:    path,
		UnsignedUpdates:  true,
		IsNewer: func(remote, local *semver.Version) bool {
			return remote.Metadata() > local.Metadata()
		},
		InstallApp: func(file string) error {
			var err error
			installed, err = os.ReadFile(file)
			return err
		},
	})
	if err := state.TryUpdate(context.Background(), UpdateParams{DeviceID: "test"}); err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(payload) || !rebooted {
		t.Fatalf("installed %q, rebooted %v", installed, rebooted)
	}
}

func TestMissingReleaseServerIsReported(t *testing.T) {
	logger := zerolog.Nop()
	state := NewState(Options{
		Logger:            &logger,
		SkipConfirmSystem: true,
		GetHTTPClient:     func() HttpClient { return http.DefaultClient },
		GetLocalVersion: func() (*semver.Version, *semver.Version, error) {
			return semver.MustParse("0.0.0"), semver.MustParse("1.0.0"), nil
		},
	})
	if _, err := state.GetUpdateStatus(context.Background(), UpdateParams{}); !errors.Is(err, ErrNoReleaseServer) {
		t.Fatalf("error %v", err)
	}
}
