func fidWhere(_ file: String = #fileID) -> String { file }

public func fileIDLiteral() {
    print(fidWhere(), #fileID)
}
