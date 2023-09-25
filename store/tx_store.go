package store

import (
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"erc20-permit-relayer/common"

	"github.com/inconshreveable/log15"
	_ "github.com/lib/pq"
)

type TxStore struct {
	db     *sql.DB
	config *common.Config
	log    log15.Logger
	mutex  sync.Mutex
}

type Tx struct {
	TxHash    string
	Payer     string
	Receiver  string
	Amount    *big.Int
	Nonce     uint64
	TxSigned  []byte
	TxNonce   uint64
	Timestamp time.Time
}

func NewTxStore(config *common.Config, log *log15.Logger) *TxStore {
	return &TxStore{
		config: config,
		log:    *log,
	}
}

func (t *TxStore) Connect() error {
	// Connect to the PostgreSQL database
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		t.config.Db.Host, t.config.Db.Port, t.config.Db.User, t.config.Db.Password, t.config.Db.Dbname)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}

	t.db = db

	// Prepare create schema
	err = t.prepareCreateSchama()
	if err != nil {
		return err
	}

	return nil
}

func (t *TxStore) Close() {
	t.db.Close()
}

// create schema
func (t *TxStore) prepareCreateSchama() error {
	// tx_pending
	createSchemaQuery := `
	CREATE TABLE IF NOT EXISTS tx_pending (
		tx_hash VARCHAR PRIMARY KEY,
		payer VARCHAR,
		receiver VARCHAR,
		amount NUMERIC,
		nonce NUMERIC,
		tx_signed BYTEA,
		tx_nonce NUMERIC,
		timestamp TIMESTAMP DEFAULT NOW()
	);`
	_, err := t.db.Exec(createSchemaQuery)
	if err != nil {
		return err
	}

	// tx_fail
	createSchemaQuery = `
	CREATE TABLE IF NOT EXISTS tx_fail (
		tx_hash VARCHAR PRIMARY KEY,
		payer VARCHAR,
		receiver VARCHAR,
		amount NUMERIC,
		nonce NUMERIC,
		tx_signed BYTEA,
		tx_nonce NUMERIC,
		timestamp TIMESTAMP,
		timestamp_fail TIMESTAMP DEFAULT NOW()
	);`
	_, err = t.db.Exec(createSchemaQuery)
	if err != nil {
		return err
	}

	// tx_submitted
	createSchemaQuery = `
	CREATE TABLE IF NOT EXISTS tx_submitted (
		tx_hash VARCHAR PRIMARY KEY,
		payer VARCHAR,
		receiver VARCHAR,
		amount NUMERIC,
		nonce NUMERIC,
		tx_signed BYTEA,
		tx_nonce NUMERIC,
		timestamp TIMESTAMP,
		timestamp_submitted TIMESTAMP DEFAULT NOW()
	);`
	_, err = t.db.Exec(createSchemaQuery)
	if err != nil {
		return err
	}

	// account_balance
	createSchemaQuery = `
	CREATE TABLE IF NOT EXISTS account_balance (
		account VARCHAR PRIMARY KEY,
		pending_balance NUMERIC,
		pending_txs NUMERIC
	);`
	_, err = t.db.Exec(createSchemaQuery)
	if err != nil {
		return err
	}

	// signer_config
	createSchemaQuery = `
	CREATE TABLE IF NOT EXISTS signer_config (
		account VARCHAR PRIMARY KEY,
		tx_nonce NUMERIC,
		timestamp TIMESTAMP DEFAULT NOW()
	);`
	_, err = t.db.Exec(createSchemaQuery)
	if err != nil {
		return err
	}

	// keeper_config
	createSchemaQuery = `
	CREATE TABLE IF NOT EXISTS keeper_config (
		instance_id VARCHAR PRIMARY KEY,
		block_number NUMERIC,
		timestamp TIMESTAMP DEFAULT NOW()
	);`
	_, err = t.db.Exec(createSchemaQuery)
	if err != nil {
		return err
	}

	return nil
}

func (t *TxStore) PrepareKeeperConfig(blockNumber int64) error {
	// Add default value if not exist
	createSchemaQuery := `
	INSERT INTO keeper_config (instance_id, block_number, timestamp)
	VALUES ($1, '0', NOW())
	ON CONFLICT (instance_id)
	DO NOTHING;`
	_, err := t.db.Exec(createSchemaQuery, t.config.Keeper.InstanceId)
	if err != nil {
		return err
	}

	// Update to InitialSyncBlockNumber
	err = t.UpdateKeeperBlockNumber(blockNumber)
	if err != nil {
		return err
	}

	return nil
}

func (t *TxStore) PrepareSignerConfig(account string) error {
	// Add default value if not exist
	createSchemaQuery := `
	INSERT INTO signer_config (account, tx_nonce, timestamp)
	VALUES ($1, '0', NOW())
	ON CONFLICT (account)
	DO NOTHING;`
	_, err := t.db.Exec(createSchemaQuery, account)
	if err != nil {
		return err
	}

	return nil
}

// tx_pending
func (t *TxStore) AddTxPending(txHash string, payer string, receiver string, amount *big.Int, nonce *big.Int, txSigned []byte, txNonce uint64) error {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	payer = strings.ToLower(payer)
	receiver = strings.ToLower(receiver)

	query := "INSERT INTO tx_pending (tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW());"
	_, err := t.db.Exec(query, txHash, payer, receiver, amount.String(), nonce.String(), txSigned, txNonce)
	if err != nil {
		return err
	}

	// Update pending balance
	err = t.updatePendingBalance(payer)
	if err != nil {
		return err
	}

	// Update pending balance
	err = t.updatePendingBalance(receiver)
	if err != nil {
		return err
	}

	return nil
}

func (t *TxStore) GetPendingBalance(account string) (*big.Int, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	account = strings.ToLower(account)

	// Get latest pending balance
	query := `SELECT pending_balance FROM account_balance WHERE account = $1`

	var result string
	err := t.db.QueryRow(query, account).Scan(&result)
	// Check not exist
	if err == sql.ErrNoRows {
		// Update pending balance
		err = t.updatePendingBalance(account)
		if err != nil {
			return nil, err
		}

		// Get latest pending balance again
		err = t.db.QueryRow(query, account).Scan(&result)
	}
	// final check query error
	if err != nil {
		return nil, err
	}

	pending_balance, _ := new(big.Int).SetString(result, 10)
	return pending_balance, nil
}

func (t *TxStore) GetPendingTxs(account string) (int64, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	account = strings.ToLower(account)

	// Get latest pending txs
	query := `SELECT pending_txs FROM account_balance WHERE account = $1`

	var result int64
	err := t.db.QueryRow(query, account).Scan(&result)
	// Check not exist
	if err == sql.ErrNoRows {
		// Update pending balance
		err = t.updatePendingBalance(account)
		if err != nil {
			return 0, err
		}

		// Get latest pending balance again
		err = t.db.QueryRow(query, account).Scan(&result)
	}
	// final check query error
	if err != nil {
		return 0, err
	}

	return result, nil
}

func (t *TxStore) GetAllTxPending(count int) ([]Tx, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if count > 0 {
		count = 1000
	}

	var txs []Tx
	query := `SELECT tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp FROM tx_pending ORDER BY tx_nonce LIMIT $1`
	rows, err := t.db.Query(query, count)
	if err != nil {
		return txs, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tx     Tx
			amount string
		)
		err := rows.Scan(&tx.TxHash, &tx.Payer, &tx.Receiver, &amount, &tx.Nonce, &tx.TxSigned, &tx.TxNonce, &tx.Timestamp)
		if err != nil {
			continue
		}

		tx.Amount, _ = new(big.Int).SetString(amount, 10)
		txs = append(txs, tx)
	}

	return txs, err
}

func (t *TxStore) GetTxPending(txHash string) (Tx, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return t.getTxPending(txHash)
}

func (t *TxStore) getTxPending(txHash string) (Tx, error) {
	var (
		tx     Tx
		amount string
	)
	query := `SELECT tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp FROM tx_pending WHERE tx_hash = $1`
	err := t.db.QueryRow(query, txHash).Scan(&tx.TxHash, &tx.Payer, &tx.Receiver, &amount, &tx.Nonce, &tx.TxSigned, &tx.TxNonce, &tx.Timestamp)
	if err != nil {
		return tx, err
	}

	tx.Amount, _ = new(big.Int).SetString(amount, 10)
	return tx, err
}

func (t *TxStore) updatePendingBalance(account string) error {
	account = strings.ToLower(account)

	// Delete before
	query := `DELETE FROM account_balance WHERE account = $1`
	_, err := t.db.Exec(query, account)
	if err != nil {
		return err
	}

	// Insert new pending_balance
	query = `
	INSERT INTO account_balance (account, pending_balance, pending_txs) 
	SELECT
		'` + account + `' AS account,
		(SELECT COALESCE(SUM(amount::NUMERIC), 0) FROM tx_pending WHERE receiver = $1) -
		(SELECT COALESCE(SUM(amount::NUMERIC), 0) FROM tx_pending WHERE payer = $1) AS pending_balance,
		(SELECT COUNT(*) FROM tx_pending WHERE payer = $1) AS pending_txs;`
	_, err = t.db.Exec(query, account)
	if err != nil {
		return err
	}
	return nil
}

// tx_submitted
func (t *TxStore) UpdateTxPendingToSubmited(txHash string) (bool, Tx, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Get payer, receiver
	tx, err := t.getTxPending(txHash)
	if err == sql.ErrNoRows {
		return false, tx, nil
	} else if err != nil {
		return false, tx, err
	}

	// Insert tx_submitted and delete tx_pending
	query := `
		WITH moved_records AS (
			INSERT INTO tx_submitted (tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp, timestamp_submitted)
			SELECT tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp, NOW()
			FROM tx_pending
			WHERE tx_hash = $1
			RETURNING tx_hash
		)
		DELETE FROM tx_pending
		WHERE tx_hash IN (SELECT tx_hash FROM moved_records);`

	_, err = t.db.Exec(query, txHash)
	if err != nil {
		return false, tx, err
	}

	// Update pending balance
	err = t.updatePendingBalance(tx.Payer)
	if err != nil {
		return false, tx, err
	}

	// Update pending balance
	err = t.updatePendingBalance(tx.Receiver)
	if err != nil {
		return false, tx, err
	}

	return true, tx, nil
}

// tx_fail
func (t *TxStore) GetAllTxFail(count int) ([]Tx, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if count > 0 {
		count = 10000
	}

	var txs []Tx
	query := `SELECT tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp FROM tx_fail ORDER BY payer, nonce LIMIT $1`
	rows, err := t.db.Query(query, count)
	if err != nil {
		return txs, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tx     Tx
			amount string
		)
		err := rows.Scan(&tx.TxHash, &tx.Payer, &tx.Receiver, &amount, &tx.Nonce, &tx.TxSigned, &tx.TxNonce, &tx.Timestamp)
		if err != nil {
			continue
		}

		tx.Amount, _ = new(big.Int).SetString(amount, 10)
		txs = append(txs, tx)
	}

	return txs, err
}

func (t *TxStore) UpdateTxPendingToFail(txHash string) (bool, Tx, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Get payer, receiver
	tx, err := t.getTxPending(txHash)
	if err == sql.ErrNoRows {
		return false, tx, nil
	} else if err != nil {
		return false, tx, err
	}

	// Insert tx_fail and delete tx_pending
	query := `
		WITH moved_records AS (
			INSERT INTO tx_fail (tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp, timestamp_fail)
			SELECT tx_hash, payer, receiver, amount, nonce, tx_signed, tx_nonce, timestamp, NOW()
			FROM tx_pending
			WHERE tx_hash = $1
			RETURNING tx_hash
		)
		DELETE FROM tx_pending
		WHERE tx_hash IN (SELECT tx_hash FROM moved_records);`

	_, err = t.db.Exec(query, txHash)
	if err != nil {
		return false, tx, err
	}

	// Update pending balance
	err = t.updatePendingBalance(tx.Payer)
	if err != nil {
		return false, tx, err
	}

	// Update pending balance
	err = t.updatePendingBalance(tx.Receiver)
	if err != nil {
		return false, tx, err
	}

	return true, tx, nil
}

// signer_config
func (t *TxStore) GetSignerTxNonce(account string) (uint64, error) {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	account = strings.ToLower(account)

	query := `SELECT tx_nonce FROM signer_config WHERE account = $1`

	var result int64
	err := t.db.QueryRow(query, account).Scan(&result)
	if err != nil {
		return 0, err
	}

	return uint64(result), nil
}

func (t *TxStore) UpdateSignerTxNonce(account string, txNonce uint64) error {
	// Ensure only one to read/write access
	t.mutex.Lock()
	defer t.mutex.Unlock()

	account = strings.ToLower(account)

	query := `
	UPDATE signer_config SET tx_nonce = $2, timestamp = NOW()
	WHERE account = $1 AND tx_nonce < $2;`
	_, err := t.db.Exec(query, account, txNonce)
	if err != nil {
		return err
	}

	return nil
}

// keeper_config
func (t *TxStore) GetKeeperBlockNumber() (int64, error) {
	// Do not t.mutex.Lock()

	query := `SELECT block_number FROM keeper_config WHERE instance_id = $1`

	var result int64
	err := t.db.QueryRow(query, t.config.Keeper.InstanceId).Scan(&result)
	if err != nil {
		return 0, err
	}

	return result, nil
}

func (t *TxStore) UpdateKeeperBlockNumber(blockNumber int64) error {
	// Do not t.mutex.Lock()

	query := `
	UPDATE keeper_config SET block_number = $2, timestamp = NOW()
	WHERE instance_id = $1 AND block_number < $2;`
	_, err := t.db.Exec(query, t.config.Keeper.InstanceId, blockNumber)
	if err != nil {
		return err
	}

	return nil
}
