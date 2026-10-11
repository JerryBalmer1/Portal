# 0.14.2: card files with non-ASCII names — report

Task: Portal.Emitters' `Push-AllCards` pushed Emitter.Terraform and got 422: `files/156 "=?utf-8?B?dGVzdHMvZml4dHVyZXMvaGNsL2VuYy3ml6XmnKwvbWFpbi50Zg==?=" must match ^[A-Za-z0-9_][A-Za-z0-9._ -]*$`. The real path is `tests/fixtures/hcl/enc-日本/main.tf`. The fix: decode encoded-word filenames on the push endpoint, and let a segment hold Unicode letters and digits. Decisions: docs/design.md 52.

## Cause

Two things failed together:
- **The encoding.** .NET's `ContentDispositionHeaderValue`, which PowerShell's multipart uses, sends a non-ASCII filename as an RFC 2047 encoded word. Go's `mime.ParseMediaType` decodes RFC 2231 (`filename*`) but hands an encoded word on as it is, so the card saw the literal `=?utf-8?B?...?=`.
- **The rule.** The segment rule was ASCII only, so even the decoded segment `enc-日本` would have been refused.

## What changed

| File | Change |
|---|---|
| `internal/server/feeds.go` | `readCard` passes each file part's filename through the new `filenameOf`. A value that is wholly encoded words (starts `=?`, ends `?=`) is decoded with `mime.WordDecoder.DecodeHeader`; any other value is the path as sent. A charset Go cannot decode is an error, so the push is a 400. |
| `internal/feed/card.go` | `segment` is now `^[\p{L}\p{Nd}_][\p{L}\p{M}\p{Nd}._ -]*$`. `checkPath` refuses a path that is not valid UTF-8, by name. The `..`, `.`, empty-segment, backslash, absolute and reserved-name checks are unchanged. |
| `internal/feed/feed_test.go` | New `TestNewCardTakesUnicodeSegments`. Taken: `enc-日本/main.tf`, Japanese, Arabic-Indic digits, NFC and NFD `café`, a space. Refused: `-` or `.` first, a combining mark first, NUL, LF, U+0085, U+200B, U+3000, `:`, invalid UTF-8, `//`, `..`. |
| `internal/server/feeds_test.go` | New `TestNonASCIIFilenamesArePushedAsTheirPath`. It pushes **the exact encoded name from the error** and **the raw UTF-8 path**: both are 201, 3 files, and the card's hash equals the hash computed over the decoded path. A malformed encoded word is 422 (left as sent, refused by the segment rule). A `koi8-r` word is 400. Both go through the spec-validating `doForm`. |
| `tests/Portal.Tests.ps1` | Feeds block, new It: it builds a card holding `tests/fixtures/hcl/enc-日本/main.tf` and asserts that .NET puts that exact encoded word on the wire. `Publish-PortalCard` then pushes it to the down feed. Result: 502 and a `failed` ledger line with 3 files, where 0.14.1 gave 422 `invalid`. The version assertions move to 0.14.2. |
| `api/openapi.yaml` | pushCard's summary states the decoding and the segment rule; `info.version` 0.14.2. `internal/gen` regenerated with `go generate ./api/...` (embedded spec only). |
| `ontology.go`, `ontology.yaml`, `src/Portal/Portal.psd1` | 0.14.2. |
| `docs/design.md` | Entry 52. |
| `CHANGELOG.md` | 0.14.2. |
| `README.md` | Use the API: the push line notes the decoding. Status leads with 0.14.2 and lists "52 is 0.14.2's". What-is-here row for `ontology.yaml`: 0.14.2. |
| `ONTOLOGY.md` | Version line 0.14.2. No Kind, property, edge or problem code changed (the `file` code's message covers the new cases). |
| `docs/manual-check-list.md` | Item **1.24** added. Item 0.2's counts brought up to date: they were 107 and 118, from 0.13.0; now 116 and 126. Verified-for line updated. |

## Choices made without asking (design.md 52)

- **Only a filename made wholly of encoded words is decoded.** That is what .NET sends. A filename with `=?...?=` in the middle is taken as sent, and the segment rule refuses `=` and `?` anyway.
- **A malformed encoded word stays as sent and is refused with 422**, which names the file. An undecodable charset is 400.
- **"Digits" means `\p{Nd}`**, decimal digits in any script. `\p{No}` and `\p{Nl}` (superscripts, ①, Ⅻ) are not taken.
- **Combining marks are taken after the first character**, so NFD names (macOS) and Indic scripts pass. A mark cannot come first.
- **No Unicode normalisation.** NFC and NFD spellings of one name are two paths. The duplicate check is still case-folded with `strings.ToLower`.
- **Unicode spaces other than U+0020, zero-width and format characters, and symbols other than `_ . -` are refused.**
- **No feed change.** Go's `archive/zip` sets the UTF-8 flag on non-ASCII names in the `.nupkg`, and the OCI layer's title annotation is a JSON string.

## Verification

- `go test ./...`: all `ok`, **116** tests (114 + 2).
- `Invoke-Build` (full): `Tests Passed: 126, Failed: 0, Skipped: 0` and `Build succeeded. 6 tasks, 0 errors, 0 warnings`; Smoke ratio 0.844.
- The first full run had **2 failures**, both fixed or explained before the green run:
  - The ontology It still matched `0\.14\.1`. I missed it in the version bump; it is fixed.
  - Boot order's "a restart boots from the newest snap" was refused with `port 18200 is in use by unknown (process 94404)`. Nothing changed for it, and it passed on the rerun. I read this as the previous block's portal not having exited yet (a race), not as something this change caused.
- Manual check **1.24** was run as written from its own `pwsh -NoProfile -File`, and its Expect is the output seen: `502 … dial tcp`, `ledger: failed Tool.Git 0.1.0 files 3`, `climb: 422 "日本/../x" is not a clean path relative to the card's folder`. No other item was rerun; they stand as verified for 0.14.1.
- Not run: Portal.Emitters' real `Push-AllCards` against a live feed. Portal.Emitters is read-only to this repo. The Pester It and check 1.24 send the same encoded word through the same .NET header class.

## Staging

Everything above is staged. `tenants/default/portal.yaml` is left unstaged as it was. Nothing is committed.
