-- atlas:delimiter $$
DROP PROCEDURE IF EXISTS `_ai_provider_model_kind_backfill`
$$
CREATE PROCEDURE `_ai_provider_model_kind_backfill`()
BEGIN
  DECLARE conflict_id BIGINT UNSIGNED DEFAULT NULL;
  DECLARE conflict_message VARCHAR(255);

  SELECT MIN(source_row.`id`) INTO conflict_id
  FROM `ai_provider_models` AS source_row
  JOIN `ai_provider_models` AS target_row
    ON target_row.`provider_id` = source_row.`provider_id`
   AND BINARY target_row.`model_id` = BINARY source_row.`model_id`
   AND target_row.`model_kind` = _ascii'image'
  WHERE source_row.`model_kind` = _ascii'chat'
    AND BINARY source_row.`model_id` = BINARY _utf8mb4'gpt-image-2';
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('gpt-image-2 provider model identity conflict: provider_model_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(agent.`id`) INTO conflict_id
  FROM `ai_agents` AS agent
  JOIN `ai_provider_models` AS model_row ON model_row.`id` = agent.`provider_model_id`
  WHERE BINARY model_row.`model_id` = BINARY _utf8mb4'gpt-image-2'
    AND (agent.`scenes_json` IS NULL
      OR JSON_TYPE(agent.`scenes_json`) <> 'ARRAY'
      OR JSON_LENGTH(agent.`scenes_json`) <> 1
      OR JSON_CONTAINS(agent.`scenes_json`, JSON_QUOTE('image_generate')) = 0);
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('gpt-image-2 agent scene conflict: agent_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(agent.`id`) INTO conflict_id
  FROM `ai_agents` AS agent
  JOIN `ai_provider_models` AS model_row ON model_row.`id` = agent.`provider_model_id`
  WHERE JSON_TYPE(agent.`scenes_json`) = 'ARRAY'
    AND JSON_CONTAINS(agent.`scenes_json`, JSON_QUOTE('image_generate')) = 1
    AND BINARY model_row.`model_id` <> BINARY _utf8mb4'gpt-image-2';
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('non-gpt-image-2 image scene conflict: agent_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(profile.`id`) INTO conflict_id
  FROM `ai_context_profiles` AS profile
  JOIN `ai_provider_models` AS model_row
    ON model_row.`id` = profile.`embedding_provider_model_id`
  WHERE model_row.`model_kind` <> _ascii'embedding';
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('profile references non-embedding model: profile_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(conflict.`embedding_provider_model_id`) INTO conflict_id
  FROM (
    SELECT `embedding_provider_model_id`
    FROM `ai_context_profiles`
    GROUP BY `embedding_provider_model_id`
    HAVING COUNT(DISTINCT CONCAT_WS(CHAR(0), `embedding_dimensions`, `embedding_max_input_tokens`, `embedding_token_counter_id`)) > 1
  ) AS conflict;
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('conflicting profile snapshots: provider_model_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  UPDATE `ai_provider_models` AS model_row
  JOIN `ai_context_profiles` AS profile
    ON profile.`embedding_provider_model_id` = model_row.`id`
  SET model_row.`embedding_dimensions` = profile.`embedding_dimensions`,
      model_row.`embedding_max_input_tokens` = profile.`embedding_max_input_tokens`,
      model_row.`embedding_token_counter_id` = profile.`embedding_token_counter_id`
  WHERE model_row.`model_kind` = _ascii'embedding';

  UPDATE `ai_provider_models`
  SET `status` = 2
  WHERE `model_kind` = _ascii'embedding'
    AND (`embedding_dimensions` IS NULL OR `embedding_max_input_tokens` IS NULL OR `embedding_token_counter_id` IS NULL);

  UPDATE `ai_provider_models`
  SET `model_kind` = _ascii'image'
  WHERE `model_kind` = _ascii'chat'
    AND BINARY `model_id` = BINARY _utf8mb4'gpt-image-2';
END
$$
CALL `_ai_provider_model_kind_backfill`()
$$
DROP PROCEDURE `_ai_provider_model_kind_backfill`
$$
