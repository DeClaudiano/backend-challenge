package wagering

import (
	"backend-challenge/internal/domain"
	domainwagering "backend-challenge/internal/domain/wagering"
)

func ResolveIdempotency(existing domainwagering.WagerTransaction, providerID, idempotencyKey string, input BusinessInput) (bool, error) {
	if existing.ProviderID() != providerID || existing.IdempotencyKey() != idempotencyKey {
		return false, domain.NewError(domain.ErrIdentityConflict, "identity does not belong to the requested provider and key")
	}
	fingerprint, err := Fingerprint(input)
	if err != nil {
		return false, err
	}
	if existing.PayloadHash() != fingerprint {
		return false, domain.NewError(domain.ErrIdentityConflict, "idempotency key was reused with different content")
	}
	return true, nil
}

func ResolveExternalIdentity(existing domainwagering.WagerTransaction, providerID, externalID, idempotencyKey string, input BusinessInput) (bool, error) {
	if existing.ProviderID() != providerID || existing.ExternalID() != externalID {
		return false, domain.NewError(domain.ErrIdentityConflict, "external identity conflict")
	}
	if existing.IdempotencyKey() != idempotencyKey {
		return false, domain.NewError(domain.ErrIdentityConflict, "external transaction cannot use another idempotency key")
	}
	return ResolveIdempotency(existing, providerID, idempotencyKey, input)
}
