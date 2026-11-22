// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "ForeignAPI",
    targets: [
        .target(name: "Ledger"),
        .target(name: "Model"),
        .target(name: "App", dependencies: ["Ledger", "Model"]),
        .testTarget(name: "AppTests", dependencies: ["App"]),
    ]
)
