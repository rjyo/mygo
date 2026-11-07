// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Language",
    platforms: [.macOS(.v13)],
    products: [.library(name: "Language", type: .dynamic, targets: ["Language"])],
    dependencies: [],
    targets: [
        .target(name: "Language"),
    ],
    swiftLanguageModes: [.v5]
)
