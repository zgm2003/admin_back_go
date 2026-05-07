-- Canonicalize the admin menu route/component for the client version page.
--
-- This migration only fixes the PAGE route metadata. Table name and button
-- permission codes are handled by 20260507_client_version_domain_rename.sql.

UPDATE `permissions`
SET `path` = '/system/clientVersion',
    `component` = 'system/clientVersion',
    `i18n_key` = 'menu.system_clientVersion',
    `updated_at` = NOW()
WHERE `platform` = 'admin'
  AND `type` = 2
  AND `is_del` = 2
  AND (
      `path` = '/system/tauriVersion'
      OR `component` = 'system/tauriVersion'
      OR `i18n_key` = 'menu.system_tauriVersion'
  );
