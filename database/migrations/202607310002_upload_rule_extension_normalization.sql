UPDATE `upload_rule`
SET
  `image_exts` = JSON_REMOVE(`image_exts`, JSON_UNQUOTE(JSON_SEARCH(`image_exts`, 'one', 'doc'))),
  `file_exts` = CASE
    WHEN JSON_SEARCH(`file_exts`, 'one', 'doc') IS NULL THEN JSON_ARRAY_APPEND(`file_exts`, '$', 'doc')
    ELSE `file_exts`
  END
WHERE JSON_SEARCH(`image_exts`, 'one', 'doc') IS NOT NULL;
