package plugin

import (
	"errors"
	"testing"

	"firestige.xyz/otus/internal/core"
)

// Test cases

func TestRegisterAndGetCapturer(t *testing.T) {
	// Clear registry before test
	capturerReg.Reset()

	// Register
	RegisterCapturer("test_cap", func() Capturer {
		return &mockCapturer{
			mockPlugin: mockPlugin{name: "test_cap"},
		}
	})

	// Get
	factory, err := GetCapturerFactory("test_cap")
	if err != nil {
		t.Fatalf("GetCapturerFactory failed: %v", err)
	}

	// Create instance
	instance := factory()
	if instance.Name() != "test_cap" {
		t.Errorf("Expected name 'test_cap', got %s", instance.Name())
	}
}

func TestRegisterAndGetParser(t *testing.T) {
	parserReg.Reset()

	RegisterParser("test_parser", func() Parser {
		return &mockParser{
			mockPlugin: mockPlugin{name: "test_parser"},
		}
	})

	factory, err := GetParserFactory("test_parser")
	if err != nil {
		t.Fatalf("GetParserFactory failed: %v", err)
	}

	instance := factory()
	if instance.Name() != "test_parser" {
		t.Errorf("Expected name 'test_parser', got %s", instance.Name())
	}
}

func TestRegisterAndGetProcessor(t *testing.T) {
	processorReg.Reset()

	RegisterProcessor("test_proc", func() Processor {
		return &mockProcessor{
			mockPlugin: mockPlugin{name: "test_proc"},
		}
	})

	factory, err := GetProcessorFactory("test_proc")
	if err != nil {
		t.Fatalf("GetProcessorFactory failed: %v", err)
	}

	instance := factory()
	if instance.Name() != "test_proc" {
		t.Errorf("Expected name 'test_proc', got %s", instance.Name())
	}
}

func TestRegisterAndGetReporter(t *testing.T) {
	reporterReg.Reset()

	RegisterReporter("test_rep", func() Reporter {
		return &mockReporter{
			mockPlugin: mockPlugin{name: "test_rep"},
		}
	})

	factory, err := GetReporterFactory("test_rep")
	if err != nil {
		t.Fatalf("GetReporterFactory failed: %v", err)
	}

	instance := factory()
	if instance.Name() != "test_rep" {
		t.Errorf("Expected name 'test_rep', got %s", instance.Name())
	}
}

func TestGetNotFoundReturnsError(t *testing.T) {
	capturerReg.Reset()
	parserReg.Reset()
	processorReg.Reset()
	reporterReg.Reset()

	// Capturer
	_, err := GetCapturerFactory("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent capturer")
	}
	if !errors.Is(err, core.ErrPluginNotFound) {
		t.Errorf("Expected ErrPluginNotFound, got %v", err)
	}

	// Parser
	_, err = GetParserFactory("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent parser")
	}
	if !errors.Is(err, core.ErrPluginNotFound) {
		t.Errorf("Expected ErrPluginNotFound, got %v", err)
	}

	// Processor
	_, err = GetProcessorFactory("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent processor")
	}
	if !errors.Is(err, core.ErrPluginNotFound) {
		t.Errorf("Expected ErrPluginNotFound, got %v", err)
	}

	// Reporter
	_, err = GetReporterFactory("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent reporter")
	}
	if !errors.Is(err, core.ErrPluginNotFound) {
		t.Errorf("Expected ErrPluginNotFound, got %v", err)
	}
}

func TestDuplicateRegisterPanics(t *testing.T) {
	capturerReg.Reset()

	// First registration
	RegisterCapturer("dup", func() Capturer {
		return &mockCapturer{mockPlugin: mockPlugin{name: "dup"}}
	})

	// Second registration should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for duplicate registration")
		}
	}()
	RegisterCapturer("dup", func() Capturer {
		return &mockCapturer{mockPlugin: mockPlugin{name: "dup"}}
	})
}

func TestEmptyNamePanics(t *testing.T) {
	capturerReg.Reset()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for empty name")
		}
	}()
	RegisterCapturer("", func() Capturer {
		return &mockCapturer{mockPlugin: mockPlugin{name: ""}}
	})
}

func TestNilFactoryPanics(t *testing.T) {
	capturerReg.Reset()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for nil factory")
		}
	}()
	RegisterCapturer("test", nil)
}

func TestList(t *testing.T) {
	// Clear and populate
	capturerReg.Reset()
	parserReg.Reset()
	processorReg.Reset()
	reporterReg.Reset()

	RegisterCapturer("cap_c", func() Capturer { return &mockCapturer{mockPlugin: mockPlugin{name: "cap_c"}} })
	RegisterCapturer("cap_a", func() Capturer { return &mockCapturer{mockPlugin: mockPlugin{name: "cap_a"}} })
	RegisterCapturer("cap_b", func() Capturer { return &mockCapturer{mockPlugin: mockPlugin{name: "cap_b"}} })

	RegisterParser("parser_z", func() Parser { return &mockParser{mockPlugin: mockPlugin{name: "parser_z"}} })
	RegisterParser("parser_x", func() Parser { return &mockParser{mockPlugin: mockPlugin{name: "parser_x"}} })

	// List should return sorted
	capList := ListCapturers()
	if len(capList) != 3 {
		t.Errorf("Expected 3 capturers, got %d", len(capList))
	}
	if capList[0] != "cap_a" || capList[1] != "cap_b" || capList[2] != "cap_c" {
		t.Errorf("Expected sorted [cap_a, cap_b, cap_c], got %v", capList)
	}

	parserList := ListParsers()
	if len(parserList) != 2 {
		t.Errorf("Expected 2 parsers, got %d", len(parserList))
	}
	if parserList[0] != "parser_x" || parserList[1] != "parser_z" {
		t.Errorf("Expected sorted [parser_x, parser_z], got %v", parserList)
	}

	// Empty lists
	procList := ListProcessors()
	if len(procList) != 0 {
		t.Errorf("Expected 0 processors, got %d", len(procList))
	}

	repList := ListReporters()
	if len(repList) != 0 {
		t.Errorf("Expected 0 reporters, got %d", len(repList))
	}
}

func TestTypeSeparation(t *testing.T) {
	// Clear all
	capturerReg.Reset()
	parserReg.Reset()
	processorReg.Reset()
	reporterReg.Reset()

	// Same name, different types should not conflict
	name := "common_name"
	RegisterCapturer(name, func() Capturer { return &mockCapturer{mockPlugin: mockPlugin{name: "cap"}} })
	RegisterParser(name, func() Parser { return &mockParser{mockPlugin: mockPlugin{name: "parser"}} })
	RegisterProcessor(name, func() Processor { return &mockProcessor{mockPlugin: mockPlugin{name: "proc"}} })
	RegisterReporter(name, func() Reporter { return &mockReporter{mockPlugin: mockPlugin{name: "rep"}} })

	// All should be retrievable
	capFactory, err := GetCapturerFactory(name)
	if err != nil {
		t.Fatalf("GetCapturerFactory failed: %v", err)
	}
	if capFactory().Name() != "cap" {
		t.Error("Capturer name mismatch")
	}

	parserFactory, err := GetParserFactory(name)
	if err != nil {
		t.Fatalf("GetParserFactory failed: %v", err)
	}
	if parserFactory().Name() != "parser" {
		t.Error("Parser name mismatch")
	}

	procFactory, err := GetProcessorFactory(name)
	if err != nil {
		t.Fatalf("GetProcessorFactory failed: %v", err)
	}
	if procFactory().Name() != "proc" {
		t.Error("Processor name mismatch")
	}

	repFactory, err := GetReporterFactory(name)
	if err != nil {
		t.Fatalf("GetReporterFactory failed: %v", err)
	}
	if repFactory().Name() != "rep" {
		t.Error("Reporter name mismatch")
	}
}
