package httpapi

import (
	"errors"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/Tobe0504/Warder/internal/domain"
	"github.com/Tobe0504/Warder/internal/store"
)

// ErrPasskeysDisabled is returned when the deployment has not been told its
// own domain, which is what a passkey is bound to.
var ErrPasskeysDisabled = errors.New("passkeys: WARDER_WEBAUTHN_RP_ID is not set")

// newWebAuthn builds the relying party from configuration.
//
// Nil, with no error, when passkeys are not configured: a deployment that does
// not know its own domain must not offer a factor it could never verify, and
// that is a configuration state rather than a failure.
func newWebAuthn(rpID, displayName string, origins []string) (*webauthn.WebAuthn, error) {
	if rpID == "" {
		return nil, nil
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("passkeys: WARDER_WEBAUTHN_RP_ID is set but WARDER_WEBAUTHN_ORIGINS is empty")
	}

	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: displayName,
		RPOrigins:     origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			// Required, not preferred: a second factor that does not verify
			// the person holding it is a second factor in name only. The
			// authenticator asks for a PIN, a fingerprint or a face.
			UserVerification: protocol.VerificationRequired,

			// Resident keys so the credential can later identify the account
			// on its own, which is what phase five's passwordless sign-in
			// needs and what costs nothing to ask for now.
			ResidentKey: protocol.ResidentKeyRequirementPreferred,
		},
	})
}

// passkeyUser adapts a Warder account to the interface the library expects.
//
// The handle is the account's uuid, not the email: it is stored inside the
// authenticator and must not change when somebody changes their address.
type passkeyUser struct {
	user     *domain.User
	passkeys []store.Passkey
}

func (u passkeyUser) WebAuthnID() []byte {
	id := u.user.ID
	return id[:]
}

func (u passkeyUser) WebAuthnName() string        { return u.user.Email }
func (u passkeyUser) WebAuthnDisplayName() string { return u.user.Name }

func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(u.passkeys))
	for _, p := range u.passkeys {
		transports := make([]protocol.AuthenticatorTransport, 0, len(p.Transports))
		for _, t := range p.Transports {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}

		out = append(out, webauthn.Credential{
			ID:              p.CredentialID,
			PublicKey:       p.PublicKey,
			AttestationType: p.AttestationType,
			Transport:       transports,
			Authenticator: webauthn.Authenticator{
				AAGUID:       p.AAGUID,
				SignCount:    uint32(p.SignCount),
				CloneWarning: false,
			},
			Flags: webauthn.CredentialFlags{
				BackupEligible: p.BackupEligible,
				BackupState:    p.BackupState,
			},
		})
	}
	return out
}

// storedPasskey converts a freshly registered credential into its stored form.
func storedPasskey(userID uuid.UUID, nickname string, c *webauthn.Credential) *store.Passkey {
	transports := make([]string, 0, len(c.Transport))
	for _, t := range c.Transport {
		transports = append(transports, string(t))
	}

	return &store.Passkey{
		UserID:          userID,
		CredentialID:    c.ID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transports:      transports,
		AAGUID:          c.Authenticator.AAGUID,
		SignCount:       int64(c.Authenticator.SignCount),
		BackupEligible:  c.Flags.BackupEligible,
		BackupState:     c.Flags.BackupState,
		Nickname:        nickname,
	}
}
