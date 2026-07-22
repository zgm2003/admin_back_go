# Auth Channel Readiness, Captcha Recovery, and Wallet Locale Design

## 1. Purpose

The Admin login page currently advertises email and phone login directly from
the platform policy, even when the corresponding delivery channel cannot send a
verification code. A user can therefore solve the slide captcha only to receive
a configuration error, after which the captcha overlay remains open. Phone
login is more serious: the auth service caches the fixed value `123456` and
returns success without invoking Tencent SMS.

The wallet page has a separate lazy-locale routing defect. `/profile/wallet`
loads the `user` locale domain, while every `wallet.*` message is owned by the
`payment` domain, so raw English message keys or fallback labels can appear on a
Chinese page.

This design closes those defects as one focused authentication and localization
change. It does not alter password login, captcha policy, session behavior, or
the external API response shape.

## 2. Approved User Behavior

1. Password login remains available whenever the platform policy enables it.
2. Email login is absent from login configuration when email verification is
   not ready for the `login` scene.
3. Phone login is absent from login configuration when SMS verification is not
   ready for the `login` scene.
4. Enabling a complete channel configuration and login template makes that
   login type appear without a frontend deployment or hard-coded switch.
5. A phone verification request generates a cryptographically random six-digit
   code, stores it with the configured TTL, and sends that exact code through
   the configured Tencent SMS sender.
6. Auth phone verification never uses a fixed fallback code. The SMS management
   page may retain its explicit deterministic test code because that code is
   confined to `TestSend` and cannot authenticate a user.
7. `captcha.required` and `captcha.invalid_or_expired` keep the captcha overlay
   open and load a fresh challenge.
8. Every other captcha-fetch or send-code error is shown to the user and then
   closes and resets the overlay. This includes missing channel configuration,
   dependency, network, timeout, contract, and internal failures.
9. `/profile/wallet` loads the payment locale domain, so Chinese wallet labels
   resolve from the existing `zh-CN/payment.ts` messages.

## 3. Scope And Non-goals

In scope:

- dynamic email and SMS readiness for login configuration;
- real SMS verification delivery and delivery logging;
- removal of the auth fixed phone code option;
- captcha overlay retry versus reset behavior in both login-specific and shared
  send-code flows;
- wallet route locale-domain selection;
- focused and full host-side tests plus runtime verification through
  `admin-dev`.

Out of scope:

- changing whether captcha is enabled or which captcha type a platform uses;
- changing password login, registration, forgot-password, session, IP capture,
  or reverse-proxy behavior;
- adding an SMS provider, credentials, or templates on the user's behalf;
- moving wallet messages between locale files;
- rebuilding the application through `admin-up` or Docker for this development
  cycle.

`admin-dev` still uses the existing Docker MySQL and Redis state services. The
restriction above concerns slow application image rebuilds, not those state
containers.

## 4. Chosen Architecture

The selected approach keeps channel knowledge inside the mail and SMS modules
and gives auth only narrow capabilities:

- each delivery service owns a readiness operation for a verification scene;
- auth owns a small readiness provider that maps login type to the matching
  channel service;
- the existing auth TTL policy remains responsible only for TTL lookup;
- auth receives a phone verification sender parallel to its existing mail
  verification sender;
- SMS owns Tencent request construction, templates, credentials, and logs.

This preserves the current module boundaries. Auth does not query mail or SMS
tables and does not learn Tencent-specific fields.

Two alternatives were rejected:

- Extending the TTL policy to include readiness and sending would combine three
  responsibilities and make the policy abstraction misleading.
- Calling `VerifyCodeTTL` from `LoginConfig` would only prove that a config row
  has a TTL. It cannot prove that a sender, enabled login template, credentials,
  app ID, or sign exists.

## 5. Channel Readiness Contract

Mail and SMS services expose a scene-aware readiness method. The auth adapter
uses it with the `login` scene for email and phone login types.

A channel is ready only when all of these conditions hold:

| Requirement | Mail | SMS |
| --- | --- | --- |
| Delivery adapter exists | Tencent mail sender | Tencent SMS sender |
| Active default config exists | Yes | Yes |
| Required config values exist | credentials, region, endpoint, sender address | credentials, region, endpoint, SDK app ID, sign name |
| Verification TTL is valid | 1-60 minutes | 1-60 minutes |
| Scene template exists and is enabled | `login` mail template | `login` SMS template |
| Provider template ID exists | Yes | Yes |
| Template data is satisfiable | `code`, `ttl_minutes` cover its variables | `code`, `ttl_minutes` cover its ordered variables |

An absent, disabled, or incomplete setup returns `ready = false` without turning
the login-config request into an error. A repository query failure or another
operational failure returns an application error; it must not be disguised as
an intentionally unavailable login method. Encrypted credential fields are
checked for presence during readiness, while actual decryption and provider
errors continue to surface from the send operation.

The auth service computes effective login types by preserving the configured
platform order and filtering only email and phone against readiness for the
`login` scene. Unknown and password types retain their existing platform-policy
handling. Code-based login uses that same `login` readiness check. The send-code
entry point instead checks the account's channel against the request's own
scene, so `forget`, `bind_phone`, and `bind_email` require their matching
templates rather than accidentally reusing login readiness. A stale client or
direct API caller therefore cannot use a channel that is not ready for the
requested operation.

Readiness is checked before asking the backend captcha verifier to consume a
challenge. This avoids wasting a valid captcha when a channel is already known
to be unavailable. A configuration change between readiness and delivery is
still handled by the normal send error path.

## 6. Verification Code Delivery Flow

For both email and phone login, auth follows one delivery sequence:

1. Normalize and validate the account, scene, login type, and effective channel.
2. Verify the submitted captcha.
3. Read the channel-owned verification TTL.
4. Generate one six-digit value with `crypto/rand`.
5. Store that value under the existing account-type, scene, and account cache
   key with the configured TTL.
6. Pass the exact same value and TTL to the mail or SMS verification sender.
7. Return success only after the channel service reports success.
8. Delete the cache entry if the sender operation fails, preserving the current
   email rollback behavior and preventing an undelivered code from being valid.

`VerifyCodeOptions.PhoneCode` and `defaultPhoneCode` are removed. The existing
code-generator injection remains available for deterministic auth unit tests,
but production always defaults to the cryptographically secure generator.

SMS adds `SendVerifyCode(ctx, scene, phone, code, ttl)`. Its implementation
shares one internal delivery pipeline with `TestSend` rather than copying the
Tencent call. The pipeline:

- validates and normalizes the phone, scene, code, and TTL;
- loads the enabled config and matching enabled template;
- supplies `code` and rounded-up `ttl_minutes` template values in template order;
- decrypts Tencent credentials;
- creates a pending SMS log using the real auth scene;
- invokes the existing `sms.Sender`;
- finalizes the log as success or failure with request ID, serial number, fee,
  duration, provider error code, and message where available.

The management `TestSend` path continues through the same pipeline with the
`test` log scene and its explicit sample code. Auth cannot call that test path.

## 7. Captcha Error State Machine

The frontend classifies errors by the stable `ApiError.code`, not by translated
message text, HTTP status alone, or substring matching. One shared predicate
recognizes only:

```text
captcha.required
captcha.invalid_or_expired
```

After a send-code failure:

| Error class | Overlay | Challenge | Pending request | Countdown |
| --- | --- | --- | --- | --- |
| Recognized captcha error | stays open | refresh | preserved | not started |
| Any other error | closes | cleared | cleared | not started |
| Success | closes | cleared | cleared | starts once |

If fetching an initial or replacement captcha fails, the blank overlay closes
and resets after the error is displayed. This prevents a modal with no usable
challenge from trapping the user.

The same rules apply in:

- `views/Login/composables/useLoginForm.ts`, which coordinates the login card;
- `components/SendCode/src/useCaptchaSendCode.ts`, which serves forgot-password
  and binding flows.

The shared predicate lives beside the captcha send-code behavior or in a small
auth-facing utility; it does not broaden the global HTTP error model.

## 8. Wallet Locale Routing

`localeDomainForPath` checks `/profile/wallet` before the generic `/profile`
branch and maps that path, including descendants, to `payment`. All other
profile routes continue to map to `user`.

No messages are duplicated or moved. The existing lazy loader merges the
selected payment domain for the active locale, and the wallet view continues to
use its existing `wallet.*` keys.

## 9. Error Handling And Compatibility

- The login-config JSON contract and login-type ordering remain unchanged.
- An unavailable channel is represented by omission from `login_type_arr`, not
  a new frontend flag.
- Operational readiness errors use the existing structured error envelope and
  fail the login-config request instead of returning misleading partial data.
- Direct send-code requests for an unavailable channel receive a stable auth
  configuration error and do not consume captcha state.
- Provider failures retain their channel-owned error and log details. Secrets,
  full credentials, and verification codes never appear in logs or errors.
- Existing cached verification keys and TTL semantics remain compatible.

## 10. Test Strategy

Backend red-green tests cover:

- login configuration filtering unavailable email and phone independently while
  retaining password and configured ordering;
- reappearance of each channel when its sender, config, TTL, credentials, and
  enabled login template are complete;
- `ready = false` for absent, disabled, or incomplete setup and propagated errors
  for repository failures;
- a direct unavailable send-code request being rejected before captcha
  consumption;
- one generated phone code being cached and passed unchanged to the SMS sender;
- removal of the cache entry when SMS delivery fails;
- no auth production path using `123456` or a `PhoneCode` option;
- SMS verification template values `code` and `ttl_minutes`, pending/success and
  pending/failure log finalization, and provider result metadata;
- platform composition wiring both readiness and real phone delivery.

Frontend red-green tests cover:

- `/profile/wallet` and descendants selecting and loading `payment`, with other
  `/profile` paths still selecting `user`;
- Chinese `wallet.*` labels resolving after route locale loading;
- both captcha error codes refreshing while preserving an open overlay;
- configuration, dependency, network, timeout, contract, and generic failures
  closing and clearing the overlay;
- initial captcha-fetch failure closing a blank overlay;
- the same behavior in the login composable and shared SendCode component;
- countdown starting only after successful delivery.

Verification uses the pinned host toolchain and the local hot-reload workflow:

1. Run focused frontend and backend tests during red-green development.
2. Run the complete host-side Go and frontend quality/test gates supported by
   the repositories.
3. Start the environment with `admin-dev`; do not rebuild application Docker
   images.
4. Verify login config, password login visibility, unavailable-channel hiding,
   captcha retry/reset behavior, and `/profile/wallet` Chinese labels in the
   live browser.
5. Where real Tencent credentials and an enabled login template are present,
   verify one phone code send and its SMS log. If external credentials are not
   configured, automated sender tests remain authoritative and the live login
   type must stay hidden.
6. Stop `admin-dev` cleanly after runtime verification, leaving Docker state
   services intact.

## 11. Acceptance Criteria

This change is complete when:

- an unconfigured mail or SMS channel is absent from login choices;
- password login remains usable;
- a configured SMS login sends a random six-digit code through Tencent and the
  delivered code is the only cached valid code;
- failed SMS delivery leaves no valid cache entry and produces a finalized
  failure log;
- non-captcha send errors close the overlay, while the two captcha errors refresh
  it in place;
- the wallet page displays its Chinese labels after a direct visit and client
  navigation;
- all specified backend and frontend tests pass;
- the live behavior is checked through `admin-dev` without an application Docker
  rebuild.
