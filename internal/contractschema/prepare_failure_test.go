package contractschema

import (
	"context"
	"strings"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/legacy/devseed"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
)

func TestUnmappedFinancialHistoryStopsPreparationWithoutChangingSource(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	var bank, batch string
	if err := p.QueryRow(ctx, `INSERT INTO finance.bank_accounts(user_id,ciphertext,key_id,last_four,status) VALUES($1,'original-encrypted-data','original-key','1234','VERIFIED') RETURNING id`, devseed.OwnerID).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO finance.batches(cutoff) VALUES(now()) RETURNING id`).Scan(&batch); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO finance.payouts(batch_id,user_id,bank_account_id,amount,status,provider_id) VALUES($1,$2,$3,85000,'UNKNOWN','original-provider-reference')`, batch, devseed.OwnerID, bank); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareLegacy(ctx, p, "yember_next_must_not_exist"); err == nil || !strings.Contains(err.Error(), "finance.payouts") {
		t.Fatal("unconverted financial records were silently omitted", err)
	}
	var intact bool
	if err := p.QueryRow(ctx, `SELECT to_regnamespace('yember_next_must_not_exist') IS NULL AND EXISTS(SELECT 1 FROM finance.payouts WHERE amount=85000 AND status='UNKNOWN' AND provider_id='original-provider-reference') AND EXISTS(SELECT 1 FROM finance.bank_accounts WHERE id=$1 AND ciphertext='original-encrypted-data'::bytea)`, bank).Scan(&intact); err != nil || !intact {
		t.Fatal("failed import changed source or left a partial schema", intact, err)
	}
}

func TestInvalidReplacementNamespaceCannotModifySource(t *testing.T) {
	p := testdb.New(t)
	for _, name := range []string{"public", "finance", "yember_next_bad; DROP SCHEMA public", "yember_next_"} {
		if _, err := PrepareLegacy(context.Background(), p, name); err == nil {
			t.Fatal("unsafe schema name accepted", name)
		}
	}
}
