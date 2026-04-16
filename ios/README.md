# OrionMobile Setup

The iOS app now vendors `SwiftTerm` locally under `ios/Vendor/SwiftTerm`, so a fresh clone does not depend on fetching that package from GitHub.

## Open the app

Use the committed project:

```bash
open ios/OrionMobile.xcodeproj
```

If the project file ever gets out of sync with `ios/project.yml`, regenerate it:

```bash
cd ios
xcodegen generate
```

## Simulator

The simulator build works out of the box with the committed bundle identifier:

```text
com.radnelson.orionmobile
```

## Physical iPhone

For device builds, select a development team in Xcode:

1. Open `OrionMobile` in Xcode.
2. Select the `OrionMobile` target.
3. Go to **Signing & Capabilities**.
4. Pick a development team.

The committed bundle identifier is hardcoded to `com.radnelson.orionmobile`. If another developer needs to install the app on their own phone and that identifier is not available to their team, they should change the bundle identifier locally before building to device.

## Notes

- If Xcode package resolution gets confused after pulling, run **File > Packages > Reset Package Caches** once, then reopen the project.
- Saved mobile connections are stored in the keychain under the app's runtime bundle identifier.
