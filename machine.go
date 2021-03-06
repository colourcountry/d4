package d4

import "io"


type MachineData struct {
    iter int64
    sample_rate float64
    save_len int
    clip float64
    controls map[string]float64
    imports func(string) (string, error)
    workers int
}

type Machine interface {
    Init(Machine) error
    Program(io.Reader) error
    Run() ([]float64, error)
    RunCode([]float64, int64) ([]float64, []float64, error)
    Fill32([]float32) error
    GetData() MachineData
    Set(string,float64) error
}
