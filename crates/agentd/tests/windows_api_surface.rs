#![cfg(windows)]

// Compile/link contract only: taking symbol addresses invokes no Windows API.
#[test]
fn pinned_windows_job_api_surface_is_available() {
    use windows_sys::Win32::{
        Foundation::CloseHandle,
        System::{
            JobObjects::{
                CreateJobObjectW, IsProcessInJob, QueryInformationJobObject,
                SetInformationJobObject, TerminateJobObject, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
            },
            Threading::{
                CreateProcessW, DeleteProcThreadAttributeList, InitializeProcThreadAttributeList,
                ResumeThread, UpdateProcThreadAttribute, WaitForSingleObject, CREATE_SUSPENDED,
                PROC_THREAD_ATTRIBUTE_HANDLE_LIST, PROC_THREAD_ATTRIBUTE_JOB_LIST,
            },
        },
    };
    let symbols = std::hint::black_box([
        CreateJobObjectW as *const (),
        QueryInformationJobObject as *const (),
        SetInformationJobObject as *const (),
        TerminateJobObject as *const (),
        CreateProcessW as *const (),
        InitializeProcThreadAttributeList as *const (),
        DeleteProcThreadAttributeList as *const (),
        UpdateProcThreadAttribute as *const (),
        IsProcessInJob as *const (),
        ResumeThread as *const (),
        WaitForSingleObject as *const (),
        CloseHandle as *const (),
    ]);
    assert!(symbols.iter().all(|symbol| !symbol.is_null()));
    assert_eq!(PROC_THREAD_ATTRIBUTE_JOB_LIST, 0x0002_000d);
    assert_eq!(PROC_THREAD_ATTRIBUTE_HANDLE_LIST, 0x0002_0002);
    assert_eq!(JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, 0x2000);
    assert_eq!(CREATE_SUSPENDED, 0x4);
}
