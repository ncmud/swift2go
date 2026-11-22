#if canImport(Darwin)
import Darwin
#else
import Glibc
#endif

public func cSignalDisposition() {
    signal(SIGUSR1, SIG_IGN)
    kill(getpid(), SIGUSR1)
    usleep(50_000)
    print("survived an ignored SIGUSR1")
    let previous = signal(SIGUSR1, SIG_DFL)
    print(previous != nil)
}
