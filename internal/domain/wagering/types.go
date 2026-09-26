package wagering

type Kind string

const (
	Opening  Kind = "OPENING"
	Bet      Kind = "BET"
	Win      Kind = "WIN"
	Loss     Kind = "LOSS"
	Refund   Kind = "REFUND"
	Rollback Kind = "ROLLBACK"
)

func (k Kind) Valid() bool {
	switch k {
	case Opening, Bet, Win, Loss, Refund, Rollback:
		return true
	}
	return false
}

type Status string

const (
	Pending          Status = "PENDING"
	PendingReference Status = "PENDING_REFERENCE"
	Processed        Status = "PROCESSED"
	Rejected         Status = "REJECTED"
	Failed           Status = "FAILED"
)

func (s Status) Terminal() bool { return s == Processed || s == Rejected || s == Failed }
func (s Status) Valid() bool {
	switch s {
	case Pending, PendingReference, Processed, Rejected, Failed:
		return true
	}
	return false
}

type Source string

const (
	Internal Source = "INTERNAL"
	External Source = "EXTERNAL"
)

type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)
