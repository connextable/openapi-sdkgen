package main

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fail("usage: verify-binary.go <target> <path>")
	}

	target, path := os.Args[1], os.Args[2]
	switch target {
	case "darwin-amd64":
		verifyMachO(path, macho.CpuAmd64)
	case "darwin-arm64":
		verifyMachO(path, macho.CpuArm64)
	case "linux-amd64":
		verifyELF(path, elf.EM_X86_64)
	case "linux-arm64":
		verifyELF(path, elf.EM_AARCH64)
	case "windows-amd64":
		verifyPE(path, pe.IMAGE_FILE_MACHINE_AMD64)
	case "windows-arm64":
		verifyPE(path, pe.IMAGE_FILE_MACHINE_ARM64)
	default:
		fail("unsupported target: %s", target)
	}
}

func verifyMachO(path string, expected macho.Cpu) {
	binary, err := macho.Open(path)
	if err != nil {
		fail("%s is not a Mach-O binary: %v", path, err)
	}
	defer binary.Close()
	if binary.Cpu != expected {
		fail("%s has Mach-O CPU %s, want %s", path, binary.Cpu, expected)
	}
}

func verifyELF(path string, expected elf.Machine) {
	binary, err := elf.Open(path)
	if err != nil {
		fail("%s is not an ELF binary: %v", path, err)
	}
	defer binary.Close()
	if binary.Machine != expected {
		fail("%s has ELF machine %s, want %s", path, binary.Machine, expected)
	}
}

func verifyPE(path string, expected uint16) {
	binary, err := pe.Open(path)
	if err != nil {
		fail("%s is not a PE binary: %v", path, err)
	}
	defer binary.Close()
	if binary.FileHeader.Machine != expected {
		fail("%s has PE machine %#x, want %#x", path, binary.FileHeader.Machine, expected)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
