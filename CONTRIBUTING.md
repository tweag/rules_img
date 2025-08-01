# Contributing to rules_img

Welcome, and thanks for considering contributing to `rules_img`.

## Contributing

There are many useful ways to help which don't involve modifying the source code:

- Improving the documentation, user-facing or internal, technical or high-level
- Reviewing changes from other contributors
- Creating, commenting, and voting on [discussions for new features][discussions]

The rest of this document is concerned with changes to the code.

## Suggesting new features and large changes

For small fixes and improvements, it is generally acceptable to open a pull request with your change.
If you find a bug and don't know how to best address it, create [an issue first][new-issue-bug].
Support for features and large changes warrant the creation of a [discussion][discussions].
This way we avoid contributors putting effort into work that is unlikely to be merged, prioritize new features and discuss how to implement complex changes.

## Resources

### Documentation

- The [README](/README.md) contains user-facing documentation, including setup in your own Bazel project and configuring authentication for different services.
- The [docs](/docs) directory contains detailed documentation on each rule, as well as a guide on choosing a [push strategy][push-strategies].

### People

`rules_img` is maintained by [Tweag][tweag]. The current project steward is Malte Poll (@malt3).

You can also [join our Discord][discord-join] server and talk to us directly.

## Setting up a development environment

Please refer to [`HACKING.md`](/docs/HACKING.md) to set up a development environment, test the code on a Bazel project and run integration tests.

[discussions]: https://github.com/tweag/rules_img/discussions
[push-strategies]: /docs/push-strategies.md
[new-issue-bug]: https://github.com/tweag/rules_img/issues/new?assignees=&labels=type%3A+bug&projects=&template=bug_report.md
[discord-join]: https://discord.gg/vYDnJYBmax
[tweag]: https://www.tweag.io/
