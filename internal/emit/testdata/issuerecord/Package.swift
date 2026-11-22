// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "IssueRecord",
    targets: [
        .target(name: "Lib"),
        .target(name: "Checks"),
        .testTarget(name: "LibTests", dependencies: ["Lib", "Checks"]),
    ]
)
