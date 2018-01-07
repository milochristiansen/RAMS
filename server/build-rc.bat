
@call mingwpath -quiet
@echo Building res.rc...

@windres -F pe-x86-64 -o res_windows_amd64.syso ./res.rc
