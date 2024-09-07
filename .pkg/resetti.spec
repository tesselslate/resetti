Name:       resetti
Version:    %{vernum}
Release:    %{rel}

Summary:    Minecraft multi-instance reset macro for speedrunning
License:    GPLv3+
URL: http://www.github.com/woofdoggo/resetti

%description
resetti is a Linux-compatible reset macro for Minecraft speedruns. It supports a variety of different resetting styles, categories, and Minecraft versions.

You can refer to the documentation (https://github.com/woofdoggo/resetti/blob/main/doc/README.md) for detailed usage instructions.

Please report any bugs which you encounter. resetti is still beta software and is not guaranteed to work.

%prep

%build
%global _missing_build_ids_terminate_build 0

%install
mkdir -p %{buildroot}/%{_bindir}
install -m 0755 out/%{name} %{buildroot}/%{_bindir}/%{name}
mkdir -p %{buildroot}/%{_datadir}/%{name}
cp ./internal/res/default.toml %{buildroot}/%{_datadir}/%{name}

%files
%{_bindir}/%{name}
%license LICENSE
%dir %{_datadir}/%{name}
%{_datadir}/%{name}/default.toml

%changelog
* Sun Jul 16 2023 Dworv <dwarvyt@gmail.com>
- Creation of RPM spec and workflow
