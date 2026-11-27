# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project aims to follow Semantic Versioning.

## [0.1.0-alpha.1](https://github.com/hop-top/wsm/compare/go/v0.1.0-alpha.0...go/v0.1.0-alpha.1) (2026-11-27)


### ⚠ BREAKING CHANGES

* **cli:** align exit codes with kit's standard table

### Features

* **access:** add --no-verify flag to bypass enforcement ([b7a8892](https://github.com/hop-top/wsm/commit/b7a8892832c5d3641f86ace3ec423e9893977ad8))
* **access:** add enforcement levels per prerequisite ([03776d3](https://github.com/hop-top/wsm/commit/03776d3c65ab318314bcbc2ae5da1f762fb34a59))
* **access:** add grant declarations and grant set ([3a5ce9b](https://github.com/hop-top/wsm/commit/3a5ce9badc2e6db4de3c7088c229bb2d1097123a))
* **agent-triage:** add identity diagnostic questions Q6-Q8 ([b938d98](https://github.com/hop-top/wsm/commit/b938d985df7dcc7188de62238ba633d78177200e))
* **agent-triage:** add identity scaffold output step ([0ed82eb](https://github.com/hop-top/wsm/commit/0ed82ebc27170d7b979e9a9b76ed00070337266b))
* **alias:** adopt kit/console/alias YAML store (T-0112) ([67024ea](https://github.com/hop-top/wsm/commit/67024ea7473f91010204003c399891a1def683e9))
* **backend:** add ListDeviceSessions query for device CLI ([e87c964](https://github.com/hop-top/wsm/commit/e87c964f3e1fbd329d04a58b796439d76628d8e4))
* **backend:** add workspace-space associations and DeleteSpace ([4286963](https://github.com/hop-top/wsm/commit/4286963e943da0c45db366d5a1b83fe4008d74c5))
* **backend:** persist workspace layout ([bc0539c](https://github.com/hop-top/wsm/commit/bc0539cae7365f88c6060c696337d1728065475e))
* **cli:** add --note|-n to state-changing wsm subcommands (T-1287) ([f373fbc](https://github.com/hop-top/wsm/commit/f373fbc62f1b40e676f59c844df4c0ab8f12f9e0))
* **cli:** add --offline/--profile/--instance globals (T-0102) ([4693348](https://github.com/hop-top/wsm/commit/4693348db944d8ea058f3e7422541111481f4ae5))
* **cli:** add aliases to all top-level commands ([4df4e6c](https://github.com/hop-top/wsm/commit/4df4e6cc55942dccf4f9f6209ebf4c0b60808329))
* **cli:** add auto-mkdir for XDG dirs and legacy migration call ([d943ffb](https://github.com/hop-top/wsm/commit/d943ffbed5375e0c41c4bf3a71f17261bc9db3d9))
* **cli:** add devices list|show and restructure snapshot into subcommands ([f2c40a7](https://github.com/hop-top/wsm/commit/f2c40a7916be303596ac4f1169de34389385ff65))
* **cli:** add version flag support ([3ee4a67](https://github.com/hop-top/wsm/commit/3ee4a67bb36d7262ca1f460466abf831805782cd))
* **cli:** add wsm init command with config dir and symlink setup ([3c8f23e](https://github.com/hop-top/wsm/commit/3c8f23e405da4d59ccc56ab55228029beb4d57ba))
* **cli:** add wsm space commands (add/list/show/remove) ([eb3236a](https://github.com/hop-top/wsm/commit/eb3236a67f929dec788b07c19cf43444915f9c6a))
* **cli:** annotate event leaves with Layer-A metadata (T-0136) ([cce1239](https://github.com/hop-top/wsm/commit/cce12396aa74336c6dcc528dd5501920a4036e6c))
* **cli:** annotate groups with Long descriptions and top-level-verb (T-0133) ([1421e66](https://github.com/hop-top/wsm/commit/1421e66c7704efdc551011c5d8d887b1a822de0c))
* **cli:** annotate misc orphan leaves with Layer-A metadata (T-0138) ([6374525](https://github.com/hop-top/wsm/commit/6374525dd2a91e9a470103099166b92d2dad42fd))
* **cli:** annotate snapshot leaves with Layer-A metadata (T-0135) ([0d14fb9](https://github.com/hop-top/wsm/commit/0d14fb938761bc6cf1034bc0fc75726f5bbe61d6))
* **cli:** annotate space and device leaves with Layer-A metadata (T-0137) ([4eb4fff](https://github.com/hop-top/wsm/commit/4eb4fffe736f5bd101bd4469722758a24c5fc96c))
* **cli:** annotate workspace leaves with Layer-A metadata (T-0134) ([0c691f3](https://github.com/hop-top/wsm/commit/0c691f34d279d9f8c28b295c760685be41b5cff5))
* **cli:** assign command groups per conventions §4 (T-0078) ([bd1fca0](https://github.com/hop-top/wsm/commit/bd1fca06d17095c314ed18057fe46848e678229d))
* **cli:** bridge --force to kit --confirm=yes on destructive verbs (T-0122) ([c28230e](https://github.com/hop-top/wsm/commit/c28230ee1ce8ad5fe9e1a85da6144df1f00e449e))
* **cli:** conform wsm command surface to kit Layer-A annotations ([e130c39](https://github.com/hop-top/wsm/commit/e130c399e26c9e9fe4a63030bb9754d430cbe090))
* **cli:** consolidate events commands into wsm events add|list|show ([cfd205e](https://github.com/hop-top/wsm/commit/cfd205e3a69465996c4bccd7e73e1c995980400e))
* **cli:** enable kit Layer-A strict validation gate (T-0123) ([6f9d641](https://github.com/hop-top/wsm/commit/6f9d6411bcbaafa93d16f3f376386264a291a2c0))
* **cli:** irc account, creds, url, channel and backfill verbs ([3f22cd5](https://github.com/hop-top/wsm/commit/3f22cd55b9e65bdd436a29909a03b185da9eac2e))
* **cli:** irc lifecycle verbs enable/disable/start/stop/restart/status/sync ([d196895](https://github.com/hop-top/wsm/commit/d1968957448bdc4a162fca96a0c970408bed7762))
* **cli:** machine-grade conformance surface ([ac458ce](https://github.com/hop-top/wsm/commit/ac458cebb89956a748ff8d7d03815721d20b466d))
* **cli:** mount kit-shipped status subcommand (T-0133) ([9ec862a](https://github.com/hop-top/wsm/commit/9ec862ab6281e5ec1cf73a75e33d700a55049ba9))
* **cli:** org-package layout flags on workspace add/update ([3a00b02](https://github.com/hop-top/wsm/commit/3a00b0212eb27d13064afc72dc44f89715f03ba3))
* **cli:** owner-managed channels and discovery control ([8959400](https://github.com/hop-top/wsm/commit/89594009b72389cca65e6fb87abf535bf4a0236a))
* **cli:** redesign to wsm &lt;resource&gt; &lt;action&gt; pattern with snapshot enhancements ([d050948](https://github.com/hop-top/wsm/commit/d050948615e3717ff9d68aa9e89c85afbf8b3d28))
* **cli:** surface discovered, peer and unmanaged channels in irc status ([faf2ac7](https://github.com/hop-top/wsm/commit/faf2ac7a3e287e634d82d3d6682839bd945f4db6))
* **cli:** wire kit/cli/config path subcommands (T-0106) ([e418cdc](https://github.com/hop-top/wsm/commit/e418cdcfee4c424d1d2f6cb0f09fb7e80d944d4b))
* **cli:** wsm irc delegate add|rm|list ([9bab402](https://github.com/hop-top/wsm/commit/9bab4027d73334ab164ac29c1e7383c1b3635f85))
* **config:** add browser profile provisioning per workspace member ([33cfd6c](https://github.com/hop-top/wsm/commit/33cfd6c1932c8a9f7f58dd8d9d1ae650bda15b11))
* **config:** add local config directory support (.wsm/ and .hop/wsm/) ([aa98f69](https://github.com/hop-top/wsm/commit/aa98f6904326db3917e042c7633e12e74a1e6e6c))
* **config:** add MigrateFromLegacy helper for ~/.wsm migration ([560cdb7](https://github.com/hop-top/wsm/commit/560cdb7fa20a264d4e9ad56fa7d0b157b43f8725))
* **config:** add XDG cache and state directory support ([6c9a952](https://github.com/hop-top/wsm/commit/6c9a952236e273d79c8e8a19f1a1dfa76e2dee9e))
* **config:** enforce mutual exclusion between .wsm/ and .hop/wsm/ ([878576c](https://github.com/hop-top/wsm/commit/878576cededa8e7ecd7749b2bdc85b40d2ced916))
* **detector:** add build/runtime builtin detectors ([d647c4d](https://github.com/hop-top/wsm/commit/d647c4dc791fe3106258990a740fed7ead11baba))
* **detector:** add dependency health detector ([e5277af](https://github.com/hop-top/wsm/commit/e5277af602037f1631c4ccda944ef6f7513aac1f))
* **detector:** add fnox integration detector (T-0009) ([bc5600a](https://github.com/hop-top/wsm/commit/bc5600ae792e7be6cd270eb42b869623a54ad85b))
* **detector:** add knowledge source detectors ([d36add2](https://github.com/hop-top/wsm/commit/d36add2b53770e61916ed3fceee24ec5bf7b706f))
* **detector:** add mise integration detector (T-0007) ([38c58ac](https://github.com/hop-top/wsm/commit/38c58aca4a59a0aa431d0ef202b5eeb553d50594))
* **detector:** add per-space and workspace-wide override support ([6c465af](https://github.com/hop-top/wsm/commit/6c465af94e75c941fc801fa45ef47c7a9ba69e62))
* **detector:** add plugin interface and registry for scanning spaces ([2e72832](https://github.com/hop-top/wsm/commit/2e728323fae15eee17d172031a22642e45369fc6))
* **detector:** add secretspec integration detector (T-0008) ([b0c21e5](https://github.com/hop-top/wsm/commit/b0c21e50dc84042ca9fc9a1da953be25bff028d5))
* **detector:** add strategy generation from detector results ([20e638d](https://github.com/hop-top/wsm/commit/20e638d9765eb0c7bb495eb57d8d9ac8d216d2dc))
* **detector:** auto-infer space type from boundary in space add ([970238f](https://github.com/hop-top/wsm/commit/970238f1206b2788e9719cdb611d18de927383bd))
* **detector:** org package marker detection ([f24c8d2](https://github.com/hop-top/wsm/commit/f24c8d2cde797b6fc0f5e1b6f5cb76e6f702e752))
* **device:** add fuzzy device resolution with ambiguity handling ([e2aed90](https://github.com/hop-top/wsm/commit/e2aed90890781b73e3dab8e51128986e279b4a23))
* **device:** add persistent machine fingerprint ([14927ab](https://github.com/hop-top/wsm/commit/14927ab0479fe70e4866ed0307de86ab8fe9b999))
* **device:** use machine fingerprint in Current() ([0b80c19](https://github.com/hop-top/wsm/commit/0b80c1952e36b31f6d4ad4b5db04814df626675e))
* **events:** publish wsm.workspace.* lifecycle topics (T-0104) ([3b6df9a](https://github.com/hop-top/wsm/commit/3b6df9a26581d0ef63efb3bafb93423fe618e52d))
* **help:** split workspace/snapshot/event into KNOWLEDGE+CURATE groups (T-0114) ([63db8b0](https://github.com/hop-top/wsm/commit/63db8b0cfbdad522934d4659bd8871a853a19f32))
* **irc:** attach refusal exit codes, guest detach e2e, sharing doc note ([72c6253](https://github.com/hop-top/wsm/commit/72c6253ed574617e668bb8e7add29948fa844a53))
* **irc:** author delegation ceilings in the host override ([08f5669](https://github.com/hop-top/wsm/commit/08f5669831b959bb203ec7ecb7aa3ede602b2371))
* **irc:** bot transport over irc-go ([2e5b523](https://github.com/hop-top/wsm/commit/2e5b523134379ff9bfbbc3b6d8448d33bebe24cc))
* **irc:** cap-aware capture to interaction.message ([fa69db2](https://github.com/hop-top/wsm/commit/fa69db22ec6c62d6fc1a0cfc905c3a9cf9b1e445))
* **irc:** channel discovery, adoption and unmanaged sweeper ([a8a6b04](https://github.com/hop-top/wsm/commit/a8a6b04e50e5a9b79c362b52d4e13aa6575a8ecd))
* **irc:** CHATHISTORY backfill with dedupe and unsupported path ([db76c89](https://github.com/hop-top/wsm/commit/db76c89ebe8fbf5ed7ee16490070cc58f7de1f0d))
* **irc:** config, profile, paths, names, ports, backoff, preflight ([767272b](https://github.com/hop-top/wsm/commit/767272bca33319466e68f38ada627558f23f290c))
* **irc:** creator members, member lists, owners, namespaces ([619ec70](https://github.com/hop-top/wsm/commit/619ec70f9d6a6bba919d4c8618147130daad66f9))
* **irc:** daemon event recorder and backfill dedupe ([a5f4e97](https://github.com/hop-top/wsm/commit/a5f4e9716ca00b30377503d85d050107508ce8fa))
* **irc:** daemon status document ([5cc924b](https://github.com/hop-top/wsm/commit/5cc924b1eeb51742a18b1bc071752257932c2b21))
* **irc:** desired channel state and provenance-tracked role diff ([a1414f1](https://github.com/hop-top/wsm/commit/a1414f1234d1b86a44fb5a279dfaf3d2d4869f5e))
* **irc:** ergo config render, managed preflight, cli wrappers ([1db3597](https://github.com/hop-top/wsm/commit/1db35974c53e38a9e33c0e33f35d27f4f65c7c9a))
* **irc:** gate irc verbs in the library, close the --force hole ([ff54fed](https://github.com/hop-top/wsm/commit/ff54fed4afedfc573d78c2113440fa906644c63a))
* **irc:** guard borrowed joins and cap guest grants at MyRole ([510bb74](https://github.com/hop-top/wsm/commit/510bb749fee9d11367baab178a0f31e6a733971b))
* **irc:** hidden irc daemon command ([86cbd8d](https://github.com/hop-top/wsm/commit/86cbd8d99c8ea878fb6213fb1500a7fa8906479d))
* **irc:** native ergo provider ([267b387](https://github.com/hop-top/wsm/commit/267b387937fde0b9b1743e8917feb49d8a28ed1b))
* **irc:** provider interfaces, caps, errors, transport, registry, resolvers ([7308e01](https://github.com/hop-top/wsm/commit/7308e0158d45fa3cf397b316ad8b9d02eb2f534c))
* **irc:** qualify creator glob by workspace, add adopt/release overrides ([02fe239](https://github.com/hop-top/wsm/commit/02fe23919aacdce5dfbb2c3c9b0533fec1637829))
* **irc:** reconcile workspace IRC state via providers ([f74ed10](https://github.com/hop-top/wsm/commit/f74ed10b2ee49f5f03170cb06d59103e64d10165))
* **irc:** secrets store, api token, bot password ([fdf9423](https://github.com/hop-top/wsm/commit/fdf94234d9368d1f9625ed65d50eb7b26d567eb9))
* **irc:** services-less plain provider ([f88316a](https://github.com/hop-top/wsm/commit/f88316a30308e1156b0504961fb33d33891f2666))
* **irc:** shared-server attach and detach ([4698323](https://github.com/hop-top/wsm/commit/469832338d58c3542c7bf2b364f6791e177b831e))
* **irc:** supervise ergo child lifecycle ([6d95ab1](https://github.com/hop-top/wsm/commit/6d95ab11dbb3764b041903fc5371466263ef6411))
* **irc:** wire --shared-with attach and guest detach into the cli ([b475466](https://github.com/hop-top/wsm/commit/b475466286287b18c90a0224d66e1af9d94d5673))
* **irc:** wire discovery into the daemon ([e5cbd02](https://github.com/hop-top/wsm/commit/e5cbd025ed55e194cde2b947102d5548676617d4))
* **irc:** wire lifecycle bus hook into the cli app context ([4a248bb](https://github.com/hop-top/wsm/commit/4a248bba191467d7fec56118f891c89a8fbdbb9f))
* **irc:** workspace irc daemon ([ded4cd3](https://github.com/hop-top/wsm/commit/ded4cd3bc2cf721a6df904b1e39b80fe1b6eb9bd))
* **irc:** workspace lifecycle bus hook for managed auto-provision ([2892cc3](https://github.com/hop-top/wsm/commit/2892cc33c527124aa5cc934546d6f1dcb0c54030))
* **log:** wire kit/log as default slog handler; stderr cleanup (T-0081) ([8b202b4](https://github.com/hop-top/wsm/commit/8b202b409236d7fa86ec1dd0bb7b84ef94ec8d1e))
* **model:** optional workspace layout attachment ([97d0a6c](https://github.com/hop-top/wsm/commit/97d0a6c26805dc4975bf252343d4049b0dafafea))
* **output:** wire device subcommands to kit/output.Render (T-0080) ([259bc4a](https://github.com/hop-top/wsm/commit/259bc4a5901745f94141a1b2c8af7f8f4de21829))
* **output:** wire event subcommands to kit/output.Render (T-0080) ([94ebfaf](https://github.com/hop-top/wsm/commit/94ebfafa0e4220dc194e91def827a60bcf49daf0))
* **output:** wire snapshot subcommands to kit/output.Render (T-0080) ([e0fa95d](https://github.com/hop-top/wsm/commit/e0fa95d3d87bfa63e495f41abdba9aa50ae989a4))
* **output:** wire space subcommands to kit/output.Render (T-0080) ([997386f](https://github.com/hop-top/wsm/commit/997386fad2c38bbb999166ef590a93b49fb0daaf))
* **output:** wire workspace list/show to kit/output.Render (T-0080) ([3b8839d](https://github.com/hop-top/wsm/commit/3b8839d974fe7c5a022f43441cf0af0b9b2617e6))
* **owsf:** add ed25519 signing key load/create and sign/verify ([f15898a](https://github.com/hop-top/wsm/commit/f15898ae09263c767eff437929ffa17b52b4aaf7))
* **owsf:** implement OWSF Core v0.1 compliance and interop ([e912844](https://github.com/hop-top/wsm/commit/e912844735763ce9de64e71d542c0f4464ddb50c))
* **owsf:** implement Profile 1 signed manifest via NewSignedManifest ([b62251d](https://github.com/hop-top/wsm/commit/b62251d319d68b2c44c2390a5f19ca604eb26c86))
* **policy:** adopt kit/runtime/policy with delete-requires-note default (T-1288) ([bafe74c](https://github.com/hop-top/wsm/commit/bafe74cdbc037f922539493affde413de3670e9c))
* **scope:** add profile/squad assignment to workspace with scopes ([77442b9](https://github.com/hop-top/wsm/commit/77442b981b1eb52b6b2b6ee0b3ebb50877c52dc8))
* **scope:** add scope intersection, union, and effective scope queries ([e3d5a4d](https://github.com/hop-top/wsm/commit/e3d5a4d87ce0f27e3440bb7e3ad0eecef88b6d02))
* **scope:** add secrets scope resolution via aps SecretStore ([e08f3d1](https://github.com/hop-top/wsm/commit/e08f3d1c38c4153bd9ab38de0cf1d3e9814a3400))
* **scope:** define scope types and ScopeSet with pattern matching ([f43cb6b](https://github.com/hop-top/wsm/commit/f43cb6b5174c88e1d16ab16aef235f174af32364))
* **scope:** enforce workspace-scoped vs space-scoped session distinction ([9ff15a5](https://github.com/hop-top/wsm/commit/9ff15a581ba3d4143628f3fb7a6e0c6262c7f798))
* **space:** disclose inference provenance on space add ([9551161](https://github.com/hop-top/wsm/commit/955116151852b2e9088d523b9abac2f1c45d6692))
* **space:** rename --boundary to --path, add rux-connect write-through ([ff34b67](https://github.com/hop-top/wsm/commit/ff34b67440791e93d1d671e460edd5eb410baa0a))
* **space:** report per-detector kind under --auto ([25a2786](https://github.com/hop-top/wsm/commit/25a278668afb34e2cf8f6f20caf4f7d2529a17e6))
* **space:** wire kit/go/core/projects into write-through ([7d98926](https://github.com/hop-top/wsm/commit/7d9892694decaafd0018ca29e07c5e70deae9e54))
* **types:** add table struct tags for kit/output (T-0079) ([9131b86](https://github.com/hop-top/wsm/commit/9131b86aadc5fb068c10824bfc8ad13fe4adb487))
* **upgrade:** integrate hop.top/upgrade into wsm ([ca93613](https://github.com/hop-top/wsm/commit/ca93613c292773f4f6ac09f1300468b694836b6e))
* **upgrade:** merge feat/upgrade into main ([dee812c](https://github.com/hop-top/wsm/commit/dee812ce5c620299466281f15ed44f6bc7459550))
* **workspace:** add member model with facet binding ([63dd71a](https://github.com/hop-top/wsm/commit/63dd71a48d2d711046b69819ea3c394bc9063b88))
* **workspace:** add workspace export as YAML ([fb56390](https://github.com/hop-top/wsm/commit/fb5639004b9ac7bf778aebe9a01da34bc6d3df68))
* **workspace:** add workspace import with conflict resolution ([238b073](https://github.com/hop-top/wsm/commit/238b073f8e5c9f5a7b439b2601e9067837f204cf))
* **workspace:** AOP v0.5 full compliance ([12122f3](https://github.com/hop-top/wsm/commit/12122f39e98e544c524436caf7d97ff583fca4b9))
* **workspace:** expose layout in manager + show ([e7a1a31](https://github.com/hop-top/wsm/commit/e7a1a3133dc8b90cff8f5870b4d8c8e360a07b31))


### Bug Fixes

* **build:** make the ui-leak check fail on a leak ([68c775b](https://github.com/hop-top/wsm/commit/68c775b7834c1f32b1a7433585802b157fbbf35c))
* **ci:** scan real files in leak leaf, reject vacuous pass ([913612a](https://github.com/hop-top/wsm/commit/913612af0c6e7662d5607543ecb37eed6c773d66))
* **cli:** adjust to kit 0.4 API drift (T-0127) ([3910c80](https://github.com/hop-top/wsm/commit/3910c80c3136ba67a197cd1acff332e7d1b7ae8b))
* **cli:** align exit codes with the exit code contract ([ddac240](https://github.com/hop-top/wsm/commit/ddac2400b89fef3f8fbbf73d03b690c3caa2be6d))
* **cli:** apply Layer-A annotation review feedback ([af48324](https://github.com/hop-top/wsm/commit/af48324205e6541b4841ba889b9e7a8547f448de))
* **cli:** classify wsm.Error kinds through kit RunE wrapper (T-1304) ([496b242](https://github.com/hop-top/wsm/commit/496b24240f544dbdd3c99d18ef53910c49fa8507))
* **cli:** drop stray top-level-verb tag from irc daemon ([68fb070](https://github.com/hop-top/wsm/commit/68fb07000872edeebb35bcce7f7b3672adcd60ac))
* **cli:** guard irc stop and restart on shared hosts ([1f67eb6](https://github.com/hop-top/wsm/commit/1f67eb6aab0b685b8b3f3a0f48a6864fe08f1e80))
* **cli:** honor NO_COLOR by clearing kit default table style ([dfa1609](https://github.com/hop-top/wsm/commit/dfa16091c4dc5aae74e89495f81d796328b529fc))
* **cli:** remap Backend exit code 5 to 64 to free unauthorized slot (T-0110) ([6bde2ad](https://github.com/hop-top/wsm/commit/6bde2ad07c1a107699cb9a3f67127caa930a4d54))
* **cli:** report resolved config in status ([0ea6fa5](https://github.com/hop-top/wsm/commit/0ea6fa5690f7d8b5101c63c228a0c339bd380c75))
* **cli:** return ErrorKindBackend exit 64 for invalid --backend value (T-0118) ([a66116c](https://github.com/hop-top/wsm/commit/a66116c61b74a285676d455310c3b15c47b3c85e))
* **cli:** unknown subcommand on group cmds exits non-zero (T-0096) ([ec26a7f](https://github.com/hop-top/wsm/commit/ec26a7fe38abb80c2c8d99116d543a44871bf20a))
* **cli:** wire workspace name completion to delete (T-0091) ([2b8081d](https://github.com/hop-top/wsm/commit/2b8081d9c1c4fe3a59760c02f2e8b5952cb9b188))
* **config:** add XDG_DATA_HOME check to defaultDataDir ([feabb13](https://github.com/hop-top/wsm/commit/feabb1398d43cbbf4d9b410ab442ed2c36f643f2))
* **config:** read .wsm/config.yaml and .hop/wsm/config.yaml (T-0108) ([99b1297](https://github.com/hop-top/wsm/commit/99b1297b2b6ea4740de4b6859910e0816ab921db))
* **config:** report actual source path in 'config show' (T-0117) ([3b6bcc0](https://github.com/hop-top/wsm/commit/3b6bcc0ca857a14885279a8669f3238b89c34a7e))
* **e2e:** deliver y/N prompt input under Linux PTY in TestWorkspaceRemove (T-0141) ([5919bfe](https://github.com/hop-top/wsm/commit/5919bfe08527ef57dcb6eb814605aa19468305a3))
* **e2e:** make PTY the child's controlling terminal ([55210b1](https://github.com/hop-top/wsm/commit/55210b15eb29b794250d9f6c8ad28abf5988ae0f))
* **e2e:** probe __start_wsm instead of _wsm_completion in interactive bash test ([05cd153](https://github.com/hop-top/wsm/commit/05cd153c88ecf090a1feede1df07aa5f1dab193a))
* **e2e:** run non-TTY commands without controlling terminal ([debc860](https://github.com/hop-top/wsm/commit/debc86033387f7928dbd07bbc9ef38a8baaa57c9))
* **irc:** bound wait on cancelled ergo subcommand ([49f5234](https://github.com/hop-top/wsm/commit/49f52347e81011a1faa3dd0b37c9405b7f6a2795))
* **irc:** grant the oper capability NS SUSPEND is gated on ([5105c5a](https://github.com/hop-top/wsm/commit/5105c5a78ffa37b42d6ec9762ca0a3f9aa4c0a3f))
* **irc:** name the generated key privkey.pem ([fc057be](https://github.com/hop-top/wsm/commit/fc057be143f753b2e83f8692f5514c1808e649f3))
* **irc:** report borrowed channels as foreign, not registered ([86f71e3](https://github.com/hop-top/wsm/commit/86f71e3d06d719a6ecbfec83cef2d3cb9980e2b2))
* **irc:** send account passphrase under the key ergo decodes ([103b3ec](https://github.com/hop-top/wsm/commit/103b3ec9fcc1de93a78334f3fdd121ecc90e1e9a))
* **space:** reject create on existing space ID ([a99e43a](https://github.com/hop-top/wsm/commit/a99e43af534ddde8d4f0a80941d3f44280b7773e))
* **workspace:** make snapshot generation idempotent for non-state-changing events ([8db405f](https://github.com/hop-top/wsm/commit/8db405fd5db46d4e034e3fb632f29bc4e9d85ea8))
* **workspace:** read-only queries create no store on first run ([f2c3eed](https://github.com/hop-top/wsm/commit/f2c3eeda2292bc4139b847f2660af704a545c860))


### Code Refactoring

* **cli:** align exit codes with kit's standard table ([7828172](https://github.com/hop-top/wsm/commit/78281725d60d6311c418dd3799341a2ff8c64e57))

## [Unreleased]

### Added
- `--note|-n` flag on `wsm workspace archive`, `wsm workspace unarchive`,
  `wsm workspace delete`, `wsm snapshot delete`, and `wsm space delete`.
  Records a transition note plumbed into ctx via
  `policy.ContextAttrsKey` for the runtime policy engine to read
  (T-1287).
- `kit/runtime/policy` adoption: declarative policy YAML at
  `$XDG_CONFIG_HOME/wsm/policies.yaml`, evaluated against state changes
  via kit's policy engine. First boot seeds the file from a bundled
  default; existing user files are never overwritten. Override via
  `WSM_POLICY_FILE` env. Default policies: `archive-requires-note`,
  `delete-requires-note`. See [`docs/wsm/policies.md`](docs/wsm/policies.md).
  (T-1288)

### Changed
- `pkg/workspace.Manager` (workspace) and snapshot manager paths now
  backed by `kit/runtime/domain.Service[Workspace]` and
  `domain.Service[Snapshot]`. Internals-only refactor; public Manager
  API and CLI behavior unchanged for callers (T-1286).
- New topic aliases: `wsm.runtime.snapshot.updated` and
  `wsm.runtime.snapshot.deleted` now publish on snapshot Update/Delete
  operations (closes the alias gap noted in kit ADR-0008 §6).

### Fixed
- `wsm space add` no longer overwrites an existing space with the same ID.
  Space IDs are global; a colliding add, in any workspace including the
  same one, fails with exit code 4 (conflict) and leaves the existing space
  and its associations unchanged.

### Note for operators
- `wsm workspace archive <id>` and `wsm workspace delete <id>` without
  `--note` now exit 4 with a `policy "<name>" denied: ...` error.
  Update scripts to pass `--note "<reason>"`. To restore pre-T-1288
  behavior temporarily, edit `$XDG_CONFIG_HOME/wsm/policies.yaml` and
  remove the offending rule. See [`docs/wsm/policies.md`](docs/wsm/policies.md)
  for migration guidance.

### Breaking
- Exit codes realigned with story 015. Capability denials now exit `6`
  (was `5`); backend / persistence failures now exit `5` (was `64`). A
  new exit `7` marks an unsatisfied prerequisite (missing binary, busy
  port, unreachable server) with the fix in the message. Scripts
  branching on `5` or `64` must be updated.
- Removed `--json` flag. Use `--format json` instead.
- Renamed `event add --format` (payload format) to `--content-format` to free
  the `--format` name for the global output selector provided by kit/cli.
- Renamed `wsm workspace remove` → `wsm workspace delete` (aliases: `rm`, `del`).
- Renamed `wsm space remove` → `wsm space delete`.
- Renamed `wsm snapshot remove` → `wsm snapshot delete` (aliases: `rm`, `del`).
- Swapped `hop.top/upgrade` for `hop.top/kit/go/core/upgrade`. Behavior parity
  for end users; agent-facing skill preamble surface preserved via
  `wsm upgrade preamble`.
- Migrated TUI to Charmbracelet bubbletea v2 (`charm.land/bubbletea/v2`,
  `charm.land/lipgloss/v2`). `wsm tui` flow unchanged for users.

### Added
- Initial project docs and story pack expansion (`001-026`).
- Decomposed implementation plans (`01a-01h`).
- Repository governance and contributor bootstrap files.
- Explicit `wsm event add <workspace> <type>` command for appending
  `message.added`, `tool_call.*`, `file.changed`, and `device.*` events.
- Explicit `wsm snapshot [workspace]` command for generating/persisting
  snapshots from CLI.
- New `pkg/tui` package with unit-tested navigation/detail/action helpers
  used by charm TUI.
- New `pkg/detector` package — plugin-based detector framework for scanning
  spaces:
  - `Detector` interface + `Registry` for pluggable detection.
  - Built-in detectors: Makefile, package.json, docker-compose, devcontainer,
    .env.example.
  - Knowledge source detectors: package registries, local docs.
  - Dependency health detector: go.mod, package.json, requirements.txt,
    Gemfile.
  - Tool integration detectors: mise (.mise.toml), SecretSpec
    (secretspec.toml), fnox (fnox.toml).
  - Strategy generation from detector results.
  - User overrides per space or workspace-wide.
- New `pkg/access` package — access grants and enforcement:
  - `Grant` / `GrantSet` for declaring subject-resource permissions.
  - `Prerequisite` with enforcement levels (required, recommended, optional).
  - `RunVerification` with `--no-verify` bypass support.
- New `pkg/scope` package — workspace scope and assignment:
  - `ScopeType` (file_patterns, operations, tool_access, secrets, network).
  - `ScopeSet` with glob-based matching for file patterns.
  - Profile/squad assignment to workspaces with scoped permissions.
  - Scope intersection, union, and effective scope query API.
  - Workspace-scoped vs space-scoped session validation.
- `pkg/config`: XDG cache (`$XDG_CACHE_HOME/wsm/`) and state
  (`$XDG_STATE_HOME/wsm/`) directory support with env/flag overrides.
- `pkg/config`: Local config directory discovery (`.wsm/` and `.hop/wsm/`)
  with walk-up search and opt-in loading.
- `pkg/workspace`: YAML workspace export (`ExportWorkspace`) with metadata
  and space definitions.
- `pkg/workspace`: YAML workspace import (`ImportWorkspace`) with conflict
  resolution strategies (skip, rename, overwrite) and dry-run support.
- `pkg/config`: `XDG_DATA_HOME` support for `defaultDataDir()`, consistent
  with cache/state dir resolution.
- `pkg/config`: Legacy migration helper (`MigrateFromLegacy`) moves
  `~/.wsm/wsm.db` to XDG data dir on first run.
- `cmd/wsm`: Auto-create XDG directories (data, cache, state) on startup.
- `pkg/device`: Persistent machine fingerprint using platform-specific
  sources (macOS IOPlatformUUID, Linux machine-id) with fallback UUID
  generation and `$XDG_DATA_HOME/wsm/machine-id` persistence.
- Story-aligned unit suites:
  - `pkg/workspace/event_test.go`
  - `pkg/workspace/snapshot_test.go`
  - expanded `pkg/workspace/manager_test.go`
  - `pkg/tui/model_test.go`
  - `pkg/tui/actions_test.go`
  - `pkg/cli/completion_test.go`

### Changed
- Plan 1 updated to include TUI in v1 with optional Charmbracelet path.
- Charm TUI now renders a master/detail layout with a dedicated detail pane that updates with selection.
- Adopted `hop.top/kit` across the CLI: `kit/console/cli` (`cli.New`,
  fang-styled help, `--help-all`, `--format`, `-V`, `-C`/`--chdir`),
  `kit/console/output` (`output.Render`, table/JSON/YAML/CSV/text), `kit/console/log`
  (slog handler), `kit/core/config.Loader`, `kit/core/xdg`, `kit/core/upgrade`,
  and `kit/tui` helpers.
- Help is now grouped: `ORGANIZE` (`workspace`, `space`, `device`),
  `CAPTURE` (`event`, `snapshot`), `INTERACT` (`init`, `tui`), and
  `MANAGEMENT` (`config`, `upgrade`, `version`, `completion`).
  `MANAGEMENT` is hidden by default; surface it with `--help-all` or
  `--help-management`.
- Bare `wsm` (no args) prints the quick-start hint to stderr (was stdout) so
  `wsm | jq` keeps stdout empty.
- Documentation: added `docs/wsm/api-cli.md` (full subcommand reference) and
  `docs/wsm/configuration.md` (config keys, env vars, layered loading).
