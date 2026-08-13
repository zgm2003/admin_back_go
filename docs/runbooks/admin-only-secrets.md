# Admin-only release secrets

Status: production operator runbook

Release evidence contains hashes and identifiers, never secret values.

## Ownership

- **Release operator** receives only the ignored backend environment file and
  artifact paths required by deployment.
- **Database operator** controls `MYSQL_DSN`, recovery storage access, and the
  database maintenance account.
- **Security owner** controls `APP_SECRET`, `REDIS_PASSWORD`, COS credentials,
  provider keys, certificates, and the destructive approval channel.

## Delivery

Secrets enter the process through the approved **environment**, an owner-only
secret manager, or an **ignored** file. Do not place values in Git, command
history, manifest JSON, proof JSON, ticket text, screenshots, or chat.

Required names include:

```text
MYSQL_DSN
APP_SECRET
REDIS_PASSWORD
COS_SECRET_ID
COS_SECRET_KEY
ADMIN_BACKEND_ENV_FILE
```

The release scripts pass credentials to child processes through environment or
standard input. They do not append them to argv and do not print them.

## Redaction check

Before attaching logs, **redact** DSNs, passwords, cookies, Authorization
headers, access/refresh credentials, provider keys, COS signatures, private
keys, and certificate content. Keep only timestamps, request IDs, release IDs,
image IDs, status codes, counts, durations, and SHA-256 values.

Run the tracked sensitive-material gate before release. A tracked dump,
non-example backend environment file, private key, certificate archive, or
credential assignment is a release failure.

## Rotation

Rotate a disclosed value at its owner, revoke the old value, restart only the
affected immutable release services, and prove health/readiness. Do not edit a
running container. Follow the session-secret rotation runbook when changing
`APP_SECRET`; active sessions must be handled explicitly.
