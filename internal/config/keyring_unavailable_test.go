package config

import (
	"context"
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/errtax"
	"github.com/zalando/go-keyring"
)

// A keyring failure that is not a clean miss — no Secret Service on the
// D-Bus (WSL, headless Linux), an unsupported platform — must surface as a
// typed keyring-unavailable error, not the raw backend message: the raw
// D-Bus text classifies as a generic validation failure and gives the user
// no way out.
func TestKeyringStoreClassifiesBackendFailures(t *testing.T) {
	keyring.MockInitWithError(errors.New("The name org.freedesktop.secrets was not provided by any .service files"))
	t.Cleanup(keyring.MockInit)

	ref := SecretRef{Profile: "default", Backend: SecretBackendKeyring, Host: "acme.atlassian.net"}
	ctx := context.Background()

	assertUnavailable := func(op string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s error = nil, want keyring-unavailable", op)
		}
		var ce *CredentialError
		if !errors.As(err, &ce) {
			t.Fatalf("%s error is not a CredentialError: %v", op, err)
		}
		if ce.ErrCode != errtax.CodeKeyringUnavailable {
			t.Fatalf("%s code = %q, want %q", op, ce.ErrCode, errtax.CodeKeyringUnavailable)
		}
	}

	_, getErr := KeyringStore{}.Get(ctx, ref)
	assertUnavailable("Get()", getErr)
	assertUnavailable("Put()", KeyringStore{}.Put(ctx, ref, "secret"))
	assertUnavailable("Delete()", KeyringStore{}.Delete(ctx, ref))
}

// A clean miss stays a credential-missing error — availability
// classification must not swallow the not-found path logout and
// transactional reads rely on.
func TestKeyringStoreMissStaysNotFound(t *testing.T) {
	keyring.MockInit()
	ref := SecretRef{Profile: "default", Backend: SecretBackendKeyring, Host: "acme.atlassian.net"}
	if _, err := (KeyringStore{}).Get(context.Background(), ref); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get() on an empty keyring = %v, want ErrCredentialNotFound", err)
	}
	if err := (KeyringStore{}).Delete(context.Background(), ref); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Delete() on an empty keyring = %v, want ErrCredentialNotFound", err)
	}
}

// KeyringAvailable answers from a probe read: a clean not-found means the
// keyring answered; a backend failure means nothing can be stored. The test
// file-store override counts as available — keyring-backed profiles never
// touch the real keyring under it.
func TestKeyringAvailable(t *testing.T) {
	t.Setenv(TestCredentialStoreDirEnv, "")

	keyring.MockInit()
	if !KeyringAvailable() {
		t.Fatal("KeyringAvailable() = false with a healthy (mock) keyring")
	}

	keyring.MockInitWithError(errors.New("no secret service"))
	t.Cleanup(keyring.MockInit)
	if KeyringAvailable() {
		t.Fatal("KeyringAvailable() = true with a failing keyring")
	}

	t.Setenv(TestCredentialStoreDirEnv, t.TempDir())
	if !KeyringAvailable() {
		t.Fatal("KeyringAvailable() = false under the test file-store override")
	}
}
