-- atlas:delimiter $$
CREATE PROCEDURE `_ai_context_assert_legacy_empty`()
BEGIN
  DECLARE legacy_rows BIGINT UNSIGNED DEFAULT 0;
  DECLARE violation_message VARCHAR(255);

  SELECT COUNT(*) INTO legacy_rows FROM `ai_knowledge_retrieval_hits`;
  IF legacy_rows <> 0 THEN
    SET violation_message = CONCAT('table=ai_knowledge_retrieval_hits, rows=', legacy_rows);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = violation_message;
  END IF;

  SELECT COUNT(*) INTO legacy_rows FROM `ai_knowledge_retrievals`;
  IF legacy_rows <> 0 THEN
    SET violation_message = CONCAT('table=ai_knowledge_retrievals, rows=', legacy_rows);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = violation_message;
  END IF;

  SELECT COUNT(*) INTO legacy_rows FROM `ai_agent_knowledge_bases`;
  IF legacy_rows <> 0 THEN
    SET violation_message = CONCAT('table=ai_agent_knowledge_bases, rows=', legacy_rows);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = violation_message;
  END IF;

  SELECT COUNT(*) INTO legacy_rows FROM `ai_knowledge_chunks`;
  IF legacy_rows <> 0 THEN
    SET violation_message = CONCAT('table=ai_knowledge_chunks, rows=', legacy_rows);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = violation_message;
  END IF;

  SELECT COUNT(*) INTO legacy_rows FROM `ai_knowledge_documents`;
  IF legacy_rows <> 0 THEN
    SET violation_message = CONCAT('table=ai_knowledge_documents, rows=', legacy_rows);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = violation_message;
  END IF;

  SELECT COUNT(*) INTO legacy_rows FROM `ai_knowledge_bases`;
  IF legacy_rows <> 0 THEN
    SET violation_message = CONCAT('table=ai_knowledge_bases, rows=', legacy_rows);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = violation_message;
  END IF;
END
$$
-- atlas:delimiter ;

CALL `_ai_context_assert_legacy_empty`();
DROP PROCEDURE `_ai_context_assert_legacy_empty`;

DROP TABLE `ai_knowledge_retrieval_hits`;
DROP TABLE `ai_knowledge_retrievals`;
DROP TABLE `ai_agent_knowledge_bases`;
DROP TABLE `ai_knowledge_chunks`;
DROP TABLE `ai_knowledge_documents`;
DROP TABLE `ai_knowledge_bases`;

ALTER TABLE `ai_provider_models`
  ALTER COLUMN `model_kind` DROP DEFAULT;
