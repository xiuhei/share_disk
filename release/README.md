# Release artifacts

This ignored tree is the single local output location for versioned,
installable artifacts:

- `android/` — signed APK/AAB files
- `ubuntu/` — Debian packages
- `windows/` — EXE/MSI installers

Only this contract is tracked. Generated artifacts are not committed; temporary
compiler and packaging state belongs under the repository-root `build/` tree.
