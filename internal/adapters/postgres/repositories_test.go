package postgres

import (
	"testing"

	"backend-challenge/internal/application/ports"
)

func TestRepositoriesImplementApplicationPorts(t *testing.T) {
	var _ ports.WalletRepository = (*WalletRepository)(nil)
	var _ ports.TransactionRepository = (*TransactionRepository)(nil)
	var _ ports.LedgerRepository = (*LedgerRepository)(nil)
	var _ ports.UnitOfWork = (*UnitOfWork)(nil)
}
