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

The simulator build works out of the box. The bundle identifier is fixed to `com.orion.mobile.simulator` for simulator builds, so no signing setup is required.

## Physical iPhone

For device builds, the bundle identifier is derived from the selected development team:

```text
com.orion.mobile.$(DEVELOPMENT_TEAM)
```

That means a new developer only needs to:

1. Open `OrionMobile` in Xcode.
2. Select the `OrionMobile` target.
3. Go to **Signing & Capabilities**.
4. Pick a development team.

Once a team is selected, Xcode can register a unique device bundle identifier without changing tracked repo files.

## Notes

- If Xcode package resolution gets confused after pulling, run **File > Packages > Reset Package Caches** once, then reopen the project.
- Saved mobile connections are stored in the keychain under the app's runtime bundle identifier, so simulator and device builds keep separate saved connections.
