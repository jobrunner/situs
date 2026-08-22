# Changelog

## [0.2.0](https://github.com/jobrunner/situs/compare/v0.1.1...v0.2.0) (2026-08-22)


### ⚠ BREAKING CHANGES

* **api:** batch habitat types by concept id, drop the runtime hostus dependency

### Features

* **api:** batch habitat types by concept id, drop the runtime hostus dependency ([011f3da](https://github.com/jobrunner/situs/commit/011f3da9484623c0270c8ad91f841479ed89a099))
* **api:** index self-description on /v1/info, and the docs sweep ([6bfbfa2](https://github.com/jobrunner/situs/commit/6bfbfa2ed690c222980803623a5d47f1f7092dde))
* **api:** mark and optionally filter species by area ([3befd7a](https://github.com/jobrunner/situs/commit/3befd7a07f52ecc5a7a9edff74308cec2f37fa4d))
* **api:** serve an offline Swagger UI at GET /docs ([532be46](https://github.com/jobrunner/situs/commit/532be4621ddf06075eb37b7019e938bbc344dfb9))
* **api:** serve an offline Swagger UI at GET /docs ([210085f](https://github.com/jobrunner/situs/commit/210085fe85c7ffa8102544e8faada83f229d709e))
* **domain:** area value object and the distribution ports ([5dd9101](https://github.com/jobrunner/situs/commit/5dd91018912f017e10aad4e5692b341ac65ea190))
* **hostus:** read species distribution per concept ([a09c730](https://github.com/jobrunner/situs/commit/a09c730a8b299b0b53da7d8284c83c739156c3d3))
* **ingest:** copy species distribution into the index ([3393d80](https://github.com/jobrunner/situs/commit/3393d80a0ee6b8e757ce36c2a595891356b8747a))
* **quality:** CodeCharta map as a third ratchet ([24d3623](https://github.com/jobrunner/situs/commit/24d362330cecd41dc346b9e875540ef88ac75be2))
* **quality:** CodeCharta-Map als dritter Ratchet ([17244bc](https://github.com/jobrunner/situs/commit/17244bc9f98c6ed73416ce03ce88e2ed5f3650fa))
* **sqlite:** species distribution table with idempotent writes and area reads ([32f8172](https://github.com/jobrunner/situs/commit/32f8172a77b25911d2d1fffca663a63b846c995b))


### Bug Fixes

* **api:** assert in_area on the batch route and reject blank concept ids ([eaa0b7c](https://github.com/jobrunner/situs/commit/eaa0b7cf3a60bf91a96fa73d96df6c6f219dd8ba))
* **api:** reject unparseable only_in_area, cover the filter wire-through, split area.go ([106ed08](https://github.com/jobrunner/situs/commit/106ed088e373cf9c49ef7c27c1a2f91239e55bde))
* **application:** fakeRepo.UpsertDistribution uses failIfNamed ([54d1f26](https://github.com/jobrunner/situs/commit/54d1f260c0f948e8312b9d80ce6abb7816b6ada7))
* **ci:** close the deferred harness minors from the branch review ([6d03d74](https://github.com/jobrunner/situs/commit/6d03d743c9e5af2637641609caa3f713cd6f2406))
* **ci:** close the deferred harness minors from the branch review ([75cfa39](https://github.com/jobrunner/situs/commit/75cfa392f81f97f2ae09aeaa322c14c79fadebda))
* **config:** default to port 8070 and make the spec server relative ([5063a39](https://github.com/jobrunner/situs/commit/5063a391f741bb2c9b5ccb2fd5f520f0a446d76f))
* **config:** default to port 8070 and make the spec server relative ([e9f802f](https://github.com/jobrunner/situs/commit/e9f802f3488c28e4229df78701b71bcdbd3d918c))
* **ingest:** move Failed to the command's report, warn on backbone drift ([334f55c](https://github.com/jobrunner/situs/commit/334f55c96780299d550b3ca0c52152ced9b2b1a7))
* **ingest:** review fixes for distribution ingest ([b1d1137](https://github.com/jobrunner/situs/commit/b1d1137a018aa1b4f8d8284eaaeefe17efd91c97))
* make the mutation gate actually generate mutants ([ee49359](https://github.com/jobrunner/situs/commit/ee4935936c1e8dac1c4debc73487b070ecae835e))
* Mutation-Gate erzeugt endlich Mutanten ([b0a838c](https://github.com/jobrunner/situs/commit/b0a838cfde63ea5f57ae61d9afa64d16b09d9e18))
* review fixes for the mutation gate ([8428288](https://github.com/jobrunner/situs/commit/84282882fe23936536240b29ba22bbfcd5d00de5))

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
