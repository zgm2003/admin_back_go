SELECT 'wallet_balance_matches_ledger' AS invariant, COUNT(*) AS violations
FROM (
  SELECT w.`id`
  FROM `user_wallets` w
  LEFT JOIN (
    SELECT `wallet_id`,
      SUM(CASE WHEN direction='in' THEN `amount_cents` ELSE -`amount_cents` END) balance,
      SUM(CASE WHEN direction='in' AND source_type='recharge' THEN `amount_cents` ELSE 0 END) recharge,
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
