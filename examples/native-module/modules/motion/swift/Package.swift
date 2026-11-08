// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Motion",
    platforms: [.macOS(.v13)],
    products: [.library(name: "Motion", type: .dynamic, targets: ["Motion"])],
    dependencies: [],
    targets: [
        .target(name: "Motion"),
    ],
    swiftLanguageModes: [.v5]
)
