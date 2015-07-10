package d4

import (
    "math"
    "fmt"
    "strconv"
    "bufio"
    "io"
)

const M_NORMAL = 0
const M_COLON = 1
const M_DEF = 2
const M_CONSTANT = 3
const M_COMMENT = 4
const M_IF_FALSE = 5

type OpcodeMachine struct {
    iter int
    sampleRate float64
    step float64
    clip float64
    code []float64
    words map[string][]string
    variables map[string][]float64
    constants map[string][]float64
    old_variables []map[string][]float64
}

func NewOpcodeMachine( sampleRate float64 ) *OpcodeMachine {
    return &OpcodeMachine{0, sampleRate, 1/(LOOP*sampleRate), 1.0, nil, nil, nil, nil, nil}
}

func (m *OpcodeMachine) Init() error {
    return nil
}

func (m *OpcodeMachine) Program( in io.Reader ) error {
    scanner := bufio.NewScanner(in)

    scanner.Split(ScanForthWords)

    var new_word string

    words := map[string][]string{ "": []string{} }
    mode := []int{M_NORMAL}

    for scanner.Scan() {
        w := scanner.Text()
        switch mode[len(mode)-1] {
            case M_COLON:
                new_word = w

                _, exists := words[new_word]
                if exists {
                    return fmt.Errorf("Scan error: %s has already been defined", new_word)
                } else {                
                    _, exists := WORDS[new_word]
                    if exists {
                        return fmt.Errorf("Scan error: %s is a built-in word and cannot be redefined", new_word)
                    } else {
                        words[new_word] = nil
                        mode[len(mode)-1] = M_DEF
                    }
                }

            case M_DEF:
                switch w {
                    case ":":
                        return fmt.Errorf("Scan error: : found inside definition")
                    case ")":
                        return fmt.Errorf("Scan error: ) found outside comment")
                    case ";":
                        mode = mode[:len(mode)-1]
                    case "(":
                        mode = append(mode, M_COMMENT)
                    default:
                        words[new_word] = append(words[new_word], w)
                }

            case M_COMMENT:
                switch w {
                    case "(":
                        mode = append(mode, M_COMMENT)
                    case ")":
                        mode = mode[:len(mode)-1]
                }

            case M_NORMAL:
                switch w {
                    case ":":
                        mode = append(mode, M_COLON)
                    case "(":
                        mode = append(mode, M_COMMENT)
                    case ";":
                        return fmt.Errorf("Scan error: ; found outside definition")
                    case ")":
                        return fmt.Errorf("Scan error: ) found outside comment")
                    default:
                        words[""] = append(words[""], w)
                }
        }
        if DEBUG {
            fmt.Println("scan",w," -- ",mode)
        }
    }

    err := scanner.Err()

    if err != nil {
        return err
    }

    m.words = words
    fmt.Println("Words: ",words)

    // We now have a set of word definitions (counting '' for everything outside a word definition)
    // which we can translate into opcodes

    var breadcrumb []string = []string{}
    var code = []float64{}

    code, err = m.compile(code, "", breadcrumb)
    
    m.code = append(code, W_EOF)
    fmt.Println("Code: ",code)

    return err
}

func (m *OpcodeMachine) compile( code []float64, word string, breadcrumb []string ) ([]float64, error) {
    var err error

    defn, ok := m.words[word]
    if ok {

        if DEBUG {
            fmt.Println(word,"=",defn,"--",breadcrumb)
        }

        // word is a defined word
        for _, outer_word := range breadcrumb {
            if outer_word == word {
                return code, fmt.Errorf("Compile error: recursive definition %s", breadcrumb)
            }
        }

        for _, w := range defn {
            word_info, ok := WORDS[w]
            if ok {
                code = append(code, word_info.opcode)
            } else {
                new_breadcrumb := append(breadcrumb, word)
                code, err = m.compile( code, w, new_breadcrumb )
                if err != nil {
                    return code, err
                }
            }
        }
    } else {

        num, err := strconv.ParseFloat(word, 64)
        if err != nil {
            return code, fmt.Errorf("Compile error: unknown word %s", word)
        }
        code = append(code, W_NUMBER)
        code = append(code, num)

    }
    return code, err
}

func (m *OpcodeMachine) Fill32( buf []float32 ) error {
    for i := range buf {

        stack, err := m.Run()

        if (err != nil) {
            return err
        }

        /* Add up whatever is left on the stack */
        r := float64(0)
        for _, s := range stack {
            r += s
        }

        /* Tweak the scale if it's clipping */
        if r > m.clip {
            m.clip = r
        }
        buf[i] = float32(r / m.clip)
    }

    return nil
}

func (m *OpcodeMachine) Run() ([]float64, error) {
    var stack []float64
    _, phase := math.Modf( float64(m.iter) * m.step )
    m.iter += 1

    var err error
    var pop float64

    code_ptr := 0
    top := -1

    mode_breadcrumb := []int{}
    mode := M_NORMAL

    w := m.code[code_ptr]

    for w != W_EOF {
        switch mode {
            case M_IF_FALSE:
                switch w {
                    case W_THEN:
                        var old_mode int
                        old_mode, mode_breadcrumb = mode_breadcrumb[len(mode_breadcrumb)-1], mode_breadcrumb[:len(mode_breadcrumb)-1]
                        mode = old_mode
                    case W_ELSE:
                        mode = M_NORMAL
                }

            case M_NORMAL:
                switch w {

                    case W_NOOP:
                        // noop
                    case W_NUMBER:
                        code_ptr += 1
                        stack = append(stack, m.code[code_ptr])
                        top += 1

                    /* Forth words */

                    case W_TRUE:
                        stack = append(stack, 1)
                        top += 1
                    case W_FALSE:
                        stack = append(stack, 0)
                        top += 1

                    case W_IF:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        mode_breadcrumb = append(mode_breadcrumb, mode)
                        if pop == 0 {
                            mode = M_IF_FALSE
                        }
                            

                    case W_THEN:
                        var old_mode int
                        old_mode, mode_breadcrumb = mode_breadcrumb[len(mode_breadcrumb)-1], mode_breadcrumb[:len(mode_breadcrumb)-1]
                        mode = old_mode

                    case W_ELSE:
                        // Must have been executing an IF clause, skip to THEN
                        mode = M_IF_FALSE

                    case W_PLUS:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] += pop
                    case W_MINUS:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] -= pop
                    case W_TIMES:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] *= pop
                    case W_DIVIDE:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] /= pop
                    case W_MOD:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] = math.Mod( stack[top], pop )
        
                    case W_EQUALS:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        if stack[top] == stack[top-1] {
                            stack = append(stack, 1)
                        } else {
                            stack = append(stack, 0)
                        }

                    case W_GREATER:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        if stack[top] > pop {
                            stack[top] = 1
                        } else {
                            stack[top] = 0
                        }

                    case W_LESS:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        if stack[top] < pop {
                            stack[top] = 1
                        } else {
                            stack[top] = 0
                        }

                    case W_NOT:
                        if stack[top] == 0 {
                            stack[top] = 1
                        } else {
                            stack[top] = 0
                        }

                    case W_AND:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        if pop != 0 && stack[top] != 0 {
                            stack[top] = 1
                        } else {
                            stack[top] = 0
                        }

                    case W_OR:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        if pop != 0 || stack[top] != 0 {
                            stack[top] = 1
                        } else {
                            stack[top] = 0
                        }

                    case W_DUP:
                        stack = append(stack, stack[top])
                        top += 1

                    case W_DDUP:
                        stack = append(stack, stack[top-1], stack[top])
                        top += 2

                    case W_OVER:
                        stack = append(stack, stack[top-1])
                        top += 1

                    case W_DROP:
                        stack = stack[:top]
                        top -= 1

                    case W_NIP:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] = pop

                    case W_TUCK:
                        stack = append(stack, stack[top])
                        top += 1
                        stack[top], stack[top-1] = stack[top-1], stack[top]

                    case W_SWAP:
                        stack[top], stack[top-1] = stack[top-1], stack[top]

                    case W_ROT:
                        stack[top], stack[top-1], stack[top-2] = stack[top-2], stack[top], stack[top-1]

                    case W_CONSTANT:
                        mode = M_CONSTANT

                    case W_LOOP:
                        // TODO 

                    /* Useful words */
        
                    case W_MAX:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] = math.Max(pop,stack[top])

                    case W_MIN:
                        pop, stack = stack[top], stack[:top]
                        top -= 1
                        stack[top] = math.Min(pop,stack[top])

                    /* musical words */

                    case W_HZ:
                        stack[top] *= SEC

                    case W_BPM:
                        stack[top] *= BPM

                    case W_S:
                        stack[top] /= SEC

                    case W_T:
                        stack = append(stack, phase)

                    case W_ON:
                        /* (time, length, base -- age, on (if on) OR off (if off) */
                        var sched, dur, now float64
                        sched, dur, now, stack = stack[top-2], stack[top-1], stack[top], stack[:top-1]
                        age := now - sched
                        if age > 0 && age < dur {
                            stack[top-2] = age
                            stack = append(stack, 1)
                            top -= 1
                        } else {
                            stack[top-2] = 0
                            top -= 2
                        }

                    /* intervals */

                    case W_SHARP:
                        stack[top] *= SEMITONE
                    case W_FLAT:
                        stack[top] /= SEMITONE
                    case W_HIGH:
                        stack[top] *= 2
                    case W_LOW:
                        stack[top] /= 2

                    /* oscillators */

                    case W_SIN:
                        stack[top] = math.Sin(stack[top] * phase * 2 * math.Pi)

                    case W_SAW:
                        stack[top] = math.Mod(stack[top] * phase * 2, 2) - 1

                    case W_TR:
                        _, frac := math.Modf(stack[top] * phase)
                        if frac < 0.5 {
                            stack[top] = frac * 4 - 1
                        } else {
                            stack[top] = 3 - frac * 4
                        }

                    case W_SQ:
                        _, frac := math.Modf(stack[top] * phase)
                        if frac < 0.5 {
                            stack[top] = 1
                        } else {
                            stack[top] = -1
                        }

                    default:
                        return stack, fmt.Errorf("Runtime error: unknown opcode %d", w)
                }
        }
        if DEBUG == true {
            fmt.Println(w,"--",stack,top,mode_breadcrumb,mode)
        }
        code_ptr += 1
        w = m.code[code_ptr]
    }
    if DEBUG == true {
        fmt.Println("<<",stack)
    }

    return stack, err
}
