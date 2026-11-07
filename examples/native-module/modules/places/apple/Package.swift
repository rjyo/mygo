// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Places",
    platforms: [.macOS(.v13)],
    products: [.library(name: "Places", type: .dynamic, targets: ["Places"])],
    dependencies: [],
    targets: [
        .target(name: "Places"),
    ],
    swiftLanguageModes: [.v5]
)
