// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "TestCycle",
    targets: [
        .target(name: "Lib"),
        .target(name: "App", dependencies: ["Lib"]),
        .testTarget(name: "LibTests", dependencies: ["Lib", "App"]),
    ]
)
