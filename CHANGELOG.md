# Changelog

## [0.1.1](https://github.com/jobrunner/situs/compare/v0.1.0...v0.1.1) (2026-08-19)


### Bug Fixes

* **release:** push the ghcr image and publish the docs on every release ([637e4b3](https://github.com/jobrunner/situs/commit/637e4b3baf3e06f432a6d9e66bdc31ba4ad96d08))
* **release:** push the ghcr image and publish the docs on every release ([8b8b67a](https://github.com/jobrunner/situs/commit/8b8b67abd2dbb15e8f7559a4a87de6b3f43bdeb3))

## 0.1.0 (2026-08-19)


### Features

* **api:** habitat-type, species and syntaxon read endpoints ([bd7abea](https://github.com/jobrunner/situs/commit/bd7abeabde8185f7c9d5c7948a3f06e2f9692242))
* **domain:** admit the approximate qualifier instead of dropping its rows ([0c6155b](https://github.com/jobrunner/situs/commit/0c6155bb905aed7b19d5c49d0c6e2dd5faf2e421))
* **domain:** typology id, habitat type key and crosswalk qualifier ([0815848](https://github.com/jobrunner/situs/commit/08158486c23803d17e3082e62db999d647f825a8))
* **i18n:** german labels plus derived entry labels from '=' annex I crosswalks ([9d48b2d](https://github.com/jobrunner/situs/commit/9d48b2d7b2afc7c009e54026b413a7cfb3a3cd0d))
* **ingest:** load typologies, habitat types, crosswalks and syntaxa from csv ([2447219](https://github.com/jobrunner/situs/commit/244721982c8e7d9506cdaa2eddf0271510d2f4e7))
* **ingest:** species roles with hostus name crosswalk and measured resolution rate ([f4e51aa](https://github.com/jobrunner/situs/commit/f4e51aa322350671cb60c33fb34cc63a5ede37c9))
* **pipeline:** convert pinned EUNIS/ESy xlsx artifacts to normalized csv ([4273975](https://github.com/jobrunner/situs/commit/42739757df986636a96157d34e7e6150cf380bda))
* situs foundation — EUNIS-Habitattypen als lokaler read-only Dienst ([6f94296](https://github.com/jobrunner/situs/commit/6f942960c5fbbdba986c59cf2fa8c46a45b3f4c7))
* **sqlite:** habitat typology schema and idempotent ingest write side ([f10f2d4](https://github.com/jobrunner/situs/commit/f10f2d453f9cefa3226a5655e42f0cd9d87156ef))


### Bug Fixes

* **api:** make the batch contract true as written and guard the row ordering ([2111c4f](https://github.com/jobrunner/situs/commit/2111c4fbdde20735245485a8aa9b62fae2bf717c))
* bind in Docker, bound the batch endpoint, gate the pipeline tests ([ebaecf6](https://github.com/jobrunner/situs/commit/ebaecf66c862966f9ff9cb4d6f95f4ffac06aae7))
* **i18n:** prove the non-overwrite invariant and make the sqlite error paths deterministic ([856e48f](https://github.com/jobrunner/situs/commit/856e48f09dcbec7c558529660913bfceab507da5))
* **ingest:** batch hostus name resolution at 50, measured against real data ([8355490](https://github.com/jobrunner/situs/commit/835549080cfcd05836a014c1b2fef9cac4bd6e3e))
* **ingest:** make the hostus batch size configurable and degrade instead of failing ([75cfc08](https://github.com/jobrunner/situs/commit/75cfc08ac7dac5dde105dc2837164be9a6119c94))
* **ingest:** pin batching/id-mapping in hostus client, close review gaps ([8f02df6](https://github.com/jobrunner/situs/commit/8f02df66544ecb1becde48ae4dfdf28e6210f615))
* **ingest:** skip wrong-field-count rows, log through configured logger, check ctx per file ([41413f2](https://github.com/jobrunner/situs/commit/41413f29ba032b171b087497b6452ee495c346e7))
* **pipeline:** stop dropping the Man-made sheet and fail loudly on header drift ([3bef233](https://github.com/jobrunner/situs/commit/3bef233b1b9eff0146e59e679e089b3383ad77c3))
* route incidental adapter logs through the configured logger and reject trailing request data ([039ce89](https://github.com/jobrunner/situs/commit/039ce89f9cec928120bf0b711993686e65bf929e))
* wire the read timeout, gate coverage in verify, test the error envelope ([f199b7a](https://github.com/jobrunner/situs/commit/f199b7afe18f00d5ec07eebb8df6c7dac984233f))
