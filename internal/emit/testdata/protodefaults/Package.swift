// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "ProtoDefaults",
    targets: [
        .target(name: "ProtoDefaults"),
        .testTarget(name: "ProtoDefaultsTests", dependencies: ["ProtoDefaults"]),
    ]
)
