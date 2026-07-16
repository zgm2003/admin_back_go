SELECT 'ai_image_soft_delete_columns' AS invariant, COUNT(*) AS violations
FROM (
  SELECT 'ai_image_tasks' table_name
  UNION ALL SELECT 'ai_image_files'
) required
LEFT JOIN information_schema.columns c
  ON c.table_schema = DATABASE()
 AND c.table_name = required.table_name
 AND c.column_name = 'is_del'
 AND c.column_type = 'tinyint'
 AND c.is_nullable = 'NO'
 AND c.column_default <=> '2'
WHERE c.column_name IS NULL;

SELECT 'ai_image_soft_delete_values' AS invariant, COUNT(*) AS violations
FROM (
  SELECT id FROM ai_image_tasks WHERE is_del NOT IN (1, 2)
  UNION ALL
  SELECT id FROM ai_image_files WHERE is_del NOT IN (1, 2)
) invalid;

SELECT 'ai_image_soft_delete_relationships' AS invariant, COUNT(*) AS violations
FROM (
  SELECT f.id
  FROM ai_image_files f
  LEFT JOIN ai_image_tasks t ON t.id = f.task_id
  WHERE f.is_del = 2
    AND (
      (t.id IS NOT NULL AND t.is_del <> 2)
      OR (
        t.id IS NULL
        AND NOT EXISTS (
          SELECT 1 FROM ai_runs r
          WHERE BINARY r.request_id = BINARY CONCAT('ai_image_task_', f.task_id)
        )
      )
    )
) invalid;
