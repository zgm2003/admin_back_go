-- COS-only upload governance.
-- Active upload runtime supports Tencent COS only. Historical OSS rows are
-- soft-disabled instead of physically deleted so old audit references remain readable.

UPDATE `upload_setting` AS s
JOIN `upload_driver` AS d ON d.`id` = s.`driver_id`
SET s.`status` = 2,
    s.`is_del` = 1,
    s.`updated_at` = NOW()
WHERE d.`driver` <> 'cos';

UPDATE `upload_driver`
SET `is_del` = 1,
    `updated_at` = NOW()
WHERE `driver` <> 'cos';

UPDATE `upload_driver`
SET `bucket_domain` = SUBSTRING_INDEX(
        SUBSTRING_INDEX(
            SUBSTRING_INDEX(
                TRIM(BOTH '/' FROM REGEXP_REPLACE(TRIM(`bucket_domain`), '^https?://', '', 1, 1, 'i')),
                '/',
                1
            ),
            '?',
            1
        ),
        '#',
        1
    ),
    `updated_at` = NOW()
WHERE `driver` = 'cos'
  AND `is_del` = 2
  AND `bucket_domain` <> '';
