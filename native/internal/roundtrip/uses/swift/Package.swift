// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Uses",
    platforms: [.macOS(.v13)],
    products: [.library(name: "Uses", type: .dynamic, targets: ["Uses"])],
    dependencies: [],
    targets: [
        .target(name: "Uses"),
    ],
    swiftLanguageModes: [.v5]
)
