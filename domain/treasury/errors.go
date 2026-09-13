package treasury

import "errors"

var (
	ErrNotFound            = errors.New("treasury: record not found")
	ErrInvalidStatus       = errors.New("treasury: invalid status transition")
	ErrBankAccountRequired = errors.New("treasury: bank_account_id is required to record a payment")
	ErrOverpayment         = errors.New("treasury: payment exceeds the outstanding balance")
)
