SELECT 'wallet_balance_matches_ledger' AS invariant, COUNT(*) AS violations
FROM (
  SELECT w.`id`
  FROM `user_wallets` w
  LEFT JOIN (
    SELECT `wallet_id`,
      SUM(CASE WHEN direction='in' THEN `amount_cents` ELSE -`amount_cents` END) balance,
      SUM(CASE WHEN direction='in' AND source_type IN ('recharge','redeem_code') THEN `amount_cents` ELSE 0 END) recharge,
      SUM(CASE WHEN direction='out' THEN `amount_cents` ELSE 0 END) consume
    FROM `wallet_transactions`
    WHERE `is_del`=2
    GROUP BY `wallet_id`
  ) x ON x.`wallet_id`=w.`id`
  WHERE w.`is_del`=2 AND (
    w.`balance_cents`<>COALESCE(x.balance,0) OR
    w.`total_recharge_cents`<>COALESCE(x.recharge,0) OR
    w.`total_consume_cents`<>COALESCE(x.consume,0)
  )
) bad;

SELECT 'redeem_code_used_without_transaction' AS invariant, COUNT(*) AS violations
FROM `redeem_codes` AS code_row
JOIN `redeem_code_batches` AS batch_row ON batch_row.`id`=code_row.`batch_id`
WHERE code_row.`state`='used'
  AND (
    (SELECT COUNT(*)
     FROM `wallet_transactions` AS transaction_row
     WHERE transaction_row.`source_type`='redeem_code'
       AND transaction_row.`source_id`=code_row.`id`
       AND transaction_row.`is_del`=2)<>1
    OR NOT EXISTS (
      SELECT 1
      FROM `wallet_transactions` AS transaction_row
      JOIN `user_wallets` AS wallet
        ON wallet.`id`=transaction_row.`wallet_id`
       AND wallet.`is_del`=2
      WHERE transaction_row.`source_type`='redeem_code'
        AND transaction_row.`source_id`=code_row.`id`
        AND transaction_row.`is_del`=2
        AND transaction_row.`user_id`=code_row.`used_by`
        AND wallet.`user_id`=code_row.`used_by`
        AND transaction_row.`amount_cents`=batch_row.`amount_cents`
        AND transaction_row.`direction`='in'
        AND transaction_row.`balance_before_cents` + transaction_row.`amount_cents`=transaction_row.`balance_after_cents`
    )
  );

SELECT 'redeem_code_transaction_without_used_code' AS invariant, COUNT(*) AS violations
FROM `wallet_transactions` AS transaction_row
LEFT JOIN `redeem_codes` AS code_row ON code_row.`id`=transaction_row.`source_id`
LEFT JOIN `redeem_code_batches` AS batch_row ON batch_row.`id`=code_row.`batch_id`
LEFT JOIN `user_wallets` AS wallet ON wallet.`id`=transaction_row.`wallet_id`
WHERE transaction_row.`source_type`='redeem_code'
  AND (
    transaction_row.`is_del`<>2
    OR code_row.`id` IS NULL
    OR code_row.`state`<>'used'
    OR code_row.`used_by` IS NULL
    OR code_row.`used_at` IS NULL
    OR batch_row.`id` IS NULL
    OR wallet.`id` IS NULL
    OR wallet.`is_del`<>2
    OR transaction_row.`user_id`<>code_row.`used_by`
    OR wallet.`user_id`<>code_row.`used_by`
    OR transaction_row.`amount_cents`<>batch_row.`amount_cents`
    OR transaction_row.`direction`<>'in'
    OR transaction_row.`balance_before_cents` + transaction_row.`amount_cents`<>transaction_row.`balance_after_cents`
  );

SELECT 'redeem_code_non_used_with_transaction' AS invariant, COUNT(*) AS violations
FROM `redeem_codes` AS code_row
WHERE code_row.`state` IN ('unused','voided')
  AND EXISTS (
    SELECT 1
    FROM `wallet_transactions` AS transaction_row
    WHERE transaction_row.`source_type`='redeem_code'
      AND transaction_row.`source_id`=code_row.`id`
  );

SELECT 'wallet_source_identity_unique' AS invariant, COUNT(*) AS violations
FROM (
  SELECT `source_type`, `source_id`
  FROM `wallet_transactions`
  WHERE `is_del`=2 AND `source_type`<>'' AND `source_id`<>0
  GROUP BY `source_type`, `source_id`
  HAVING COUNT(*)>1
) bad;

SELECT 'payment_callback_identity_unique' AS invariant, COUNT(*) AS violations
FROM (
  SELECT `provider`, `notify_id`
  FROM `payment_callback_events`
  WHERE `is_del`=2 AND `notify_id`<>''
  GROUP BY `provider`, `notify_id`
  HAVING COUNT(*)>1
) bad;
