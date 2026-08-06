@_silgen_name("threadline_client_ffi_contract_version")
private func threadlineClientFFIContractVersion() -> UInt32

public enum ThreadlineIOSHostSkeleton {
    public static var bridgeContractVersion: UInt32 {
        threadlineClientFFIContractVersion()
    }
}
