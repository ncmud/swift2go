private enum MpcbAbility: String { case armor, poison, fire }

private enum MpcbCause {
    case weapon(String)
    case ability(MpcbAbility)
    case condition(MpcbAbility)
    case reflected(MpcbAbility)
}

private func mpcbNoun(_ cause: MpcbCause, periodic: Bool) -> String {
    switch cause {
    case .weapon(let attack): return attack
    case .ability(let ability) where periodic, .condition(let ability): return "\(ability.rawValue) tick"
    case .ability(let ability): return ability.rawValue
    case .reflected(let ability): return "reflected \(ability.rawValue)"
    }
}

public func multiPatternCaseBindings() {
    print(mpcbNoun(.ability(.poison), periodic: true))
    print(mpcbNoun(.ability(.poison), periodic: false))
    print(mpcbNoun(.condition(.fire), periodic: false))
    print(mpcbNoun(.reflected(.armor), periodic: true))
    print(mpcbNoun(.weapon("stab"), periodic: true))
}
