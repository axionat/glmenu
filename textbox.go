package glmenu

import (
	"github.com/4ydx/gltext/v4.1"
	"github.com/axionat/glfw/v3.2/glfw"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"time"
)

var textboxVertexShader string = `
#version 330

uniform mat4 orthographic_matrix;
uniform vec2 final_position;

in vec4 centered_position;

void main() {
  vec4 center = orthographic_matrix * centered_position;
  gl_Position = vec4(center.x + final_position.x, center.y + final_position.y, center.z, center.w);
}
` + "\x00"

var textboxFragmentShader string = `
#version 330

uniform vec3 background;
out vec4 fragment_color;

void main() {
	fragment_color = vec4(background, 1);
}
` + "\x00"

type TextBoxInteraction func(
	textbox *TextBox,
	xPos, yPos float64,
	button MouseClick,
	isInBoundingBox bool,
)

type TextBox struct {
	Menu               *Menu
	Text               *v41.Text
	Cursor             *v41.Text
	CursorIndex        int   // position of the cursor within the text
	CursorBarFrequency int64 // how long does each flash cycle last (visible -> invisible -> visible)
	MaxLength          int
	Time               time.Time
	IsEdit             bool
	IsClick            bool

	// user defined
	OnClick    TextBoxInteraction
	OnRelease  TextBoxInteraction
	FilterRune func(r rune) bool

	// opengl oriented
	program          uint32
	glMatrix         int32
	position         uint32
	vao              uint32
	vbo              uint32
	ebo              uint32
	vboData          []float32
	vboIndexCount    int
	eboData          []int32
	eboIndexCount    int
	centeredPosition uint32

	backgroundUniform         int32
	borderBackground          mgl32.Vec3
	textBackground            mgl32.Vec3
	finalPositionUniform      int32
	finalPosition             mgl32.Vec2
	orthographicMatrixUniform int32

	// X1, X2: the lower left and upper right points of a box that bounds the text
	X1          Point
	X2          Point
	BorderWidth int32
	height      float32
	width       float32

	Position mgl32.Vec2
}

func (textbox *TextBox) Height() float32 {
	return textbox.height
}

func (textbox *TextBox) Width() float32 {
	return textbox.width
}

func (textbox *TextBox) GetPosition() mgl32.Vec2 {
	return textbox.Text.Position
}

func (textbox *TextBox) GetPadding() Padding {
	return Padding{}
}

func (textbox *TextBox) Load(menu *Menu, width, height float32, borderWidth int32) (err error) {
	textbox.Menu = menu

	// text
	textbox.CursorBarFrequency = time.Duration.Nanoseconds(500000000)
	textbox.Text = v41.NewText(menu.Font, 1.0, 1.1)
	textbox.Cursor = v41.NewText(menu.Font, 1.0, 1.1)
	textbox.Cursor.SetString("|")

	// border formatting
	textbox.BorderWidth = borderWidth
	textbox.height = height
	textbox.width = width
	textbox.X1.X = -float32(width) / 2.0
	textbox.X1.Y = -float32(height) / 2.0
	textbox.X2.X = float32(width) / 2.0
	textbox.X2.Y = float32(height) / 2.0
	textbox.borderBackground = mgl32.Vec3{1.0, 1.0, 1.0}
	textbox.textBackground = mgl32.Vec3{0.0, 0.0, 0.0}

	// create shader program and define attributes and uniforms
	textbox.program, err = v41.NewProgram(textboxVertexShader, textboxFragmentShader)
	if err != nil {
		return err
	}

	// ebo, vbo data
	// 4 border edges (4 vertices apiece with 2 position points per index)
	// 1 background square (4 vertices x 2 positions)
	textbox.vboIndexCount = 4*4*2 + 1*4*2
	textbox.eboIndexCount = 4*6 + 1*6
	textbox.vboData = make([]float32, textbox.vboIndexCount, textbox.vboIndexCount)
	textbox.eboData = make([]int32, textbox.eboIndexCount, textbox.eboIndexCount)
	textbox.makeBufferData()

	// attributes
	textbox.centeredPosition = uint32(gl.GetAttribLocation(textbox.program, gl.Str("centered_position\x00")))

	// uniforms
	textbox.backgroundUniform = gl.GetUniformLocation(textbox.program, gl.Str("background\x00"))
	textbox.finalPositionUniform = gl.GetUniformLocation(textbox.program, gl.Str("final_position\x00"))
	textbox.orthographicMatrixUniform = gl.GetUniformLocation(textbox.program, gl.Str("orthographic_matrix\x00"))

	gl.GenVertexArrays(1, &textbox.vao)
	gl.GenBuffers(1, &textbox.vbo)
	gl.GenBuffers(1, &textbox.ebo)

	glfloatSize := int32(4)

	// vao
	gl.BindVertexArray(textbox.vao)

	// vbo
	gl.BindBuffer(gl.ARRAY_BUFFER, textbox.vbo)

	gl.EnableVertexAttribArray(textbox.centeredPosition)
	gl.VertexAttribPointer(
		textbox.centeredPosition,
		2,
		gl.FLOAT,
		false,
		0,
		gl.PtrOffset(0),
	)
	gl.BufferData(gl.ARRAY_BUFFER, int(glfloatSize)*textbox.vboIndexCount, gl.Ptr(textbox.vboData), gl.DYNAMIC_DRAW)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, textbox.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, int(glfloatSize)*textbox.eboIndexCount, gl.Ptr(textbox.eboData), gl.DYNAMIC_DRAW)
	gl.BindVertexArray(0)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, 0)
	return
}

// X1: lower left hand point
// X2: upper right hand point
// using the X1 and X2 values a border is built using 4 quads: left, right, top and bottom
// - left and right quads expand above and below based on the border width
// - top and bottom quads horizontal width does not include the border width
func (textbox *TextBox) makeBufferData() {
	// this all works because the original positioning is centered around the origin

	// left border edge
	// vbo data (ebo data value)
	// 0,1 (0) -> first  point of the quad which is drawn CCW
	// 2,3 (1) -> second point
	// 4,5 (2) -> third  point
	// 6,7 (3) -> fourth point
	// one triangle is drawn using 0,1,2 and the next using 0,2,3 - this pattern applies to all edges (left, right, top, bottom)
	textbox.vboData[0] = textbox.X1.X
	textbox.vboData[1] = textbox.X2.Y + float32(textbox.BorderWidth)
	textbox.vboData[2] = textbox.X1.X - float32(textbox.BorderWidth)
	textbox.vboData[3] = textbox.X2.Y + float32(textbox.BorderWidth)
	textbox.vboData[4] = textbox.X1.X - float32(textbox.BorderWidth)
	textbox.vboData[5] = textbox.X1.Y - float32(textbox.BorderWidth)
	textbox.vboData[6] = textbox.X1.X
	textbox.vboData[7] = textbox.X1.Y - float32(textbox.BorderWidth)
	textbox.eboData[0], textbox.eboData[1], textbox.eboData[2], textbox.eboData[3], textbox.eboData[4], textbox.eboData[5] = 0, 1, 2, 0, 2, 3

	// top border edge - intentionally leaves out the borderwidth on the x-axis
	textbox.vboData[8] = textbox.X2.X
	textbox.vboData[9] = textbox.X2.Y + float32(textbox.BorderWidth)
	textbox.vboData[10] = textbox.X1.X
	textbox.vboData[11] = textbox.X2.Y + float32(textbox.BorderWidth)
	textbox.vboData[12] = textbox.X1.X
	textbox.vboData[13] = textbox.X2.Y
	textbox.vboData[14] = textbox.X2.X
	textbox.vboData[15] = textbox.X2.Y
	textbox.eboData[6], textbox.eboData[7], textbox.eboData[8], textbox.eboData[9], textbox.eboData[10], textbox.eboData[11] = 4, 5, 6, 4, 6, 7

	// bottom border edge - intentionally leaves out the borderwidth on the x-axis
	textbox.vboData[16] = textbox.X2.X
	textbox.vboData[17] = textbox.X1.Y
	textbox.vboData[18] = textbox.X1.X
	textbox.vboData[19] = textbox.X1.Y
	textbox.vboData[20] = textbox.X1.X
	textbox.vboData[21] = textbox.X1.Y - float32(textbox.BorderWidth)
	textbox.vboData[22] = textbox.X2.X
	textbox.vboData[23] = textbox.X1.Y - float32(textbox.BorderWidth)
	textbox.eboData[12], textbox.eboData[13], textbox.eboData[14], textbox.eboData[15], textbox.eboData[16], textbox.eboData[17] = 8, 9, 10, 8, 10, 11

	// right border edge
	textbox.vboData[24] = textbox.X2.X + float32(textbox.BorderWidth)
	textbox.vboData[25] = textbox.X2.Y + float32(textbox.BorderWidth)
	textbox.vboData[26] = textbox.X2.X
	textbox.vboData[27] = textbox.X2.Y + float32(textbox.BorderWidth)
	textbox.vboData[28] = textbox.X2.X
	textbox.vboData[29] = textbox.X1.Y - float32(textbox.BorderWidth)
	textbox.vboData[30] = textbox.X2.X + float32(textbox.BorderWidth)
	textbox.vboData[31] = textbox.X1.Y - float32(textbox.BorderWidth)
	textbox.eboData[18], textbox.eboData[19], textbox.eboData[20], textbox.eboData[21], textbox.eboData[22], textbox.eboData[23] = 12, 13, 14, 12, 14, 15

	// background
	textbox.vboData[32] = textbox.X2.X
	textbox.vboData[33] = textbox.X2.Y
	textbox.vboData[34] = textbox.X1.X
	textbox.vboData[35] = textbox.X2.Y
	textbox.vboData[36] = textbox.X1.X
	textbox.vboData[37] = textbox.X1.Y
	textbox.vboData[38] = textbox.X2.X
	textbox.vboData[39] = textbox.X1.Y
	textbox.eboData[24], textbox.eboData[25], textbox.eboData[26], textbox.eboData[27], textbox.eboData[28], textbox.eboData[29] = 16, 17, 18, 16, 18, 19
}

func (textbox *TextBox) SetColor(color mgl32.Vec3) {
	textbox.Text.SetColor(color)
	textbox.Cursor.SetColor(color)
}

func (textbox *TextBox) SetString(str string, argv ...interface{}) {
	if len(argv) == 0 {
		textbox.Text.SetString(str)
	} else {
		textbox.Text.SetString(str, argv)
	}
}

func (textbox *TextBox) Draw() {
	if time.Since(textbox.Time).Nanoseconds() > textbox.CursorBarFrequency {
		if textbox.Cursor.RuneCount == 0 && textbox.IsEdit {
			textbox.Cursor.RuneCount = 1
		} else {
			textbox.Cursor.RuneCount = 0
		}
		textbox.Time = time.Now()
	}
	gl.UseProgram(textbox.program)

	// draw
	gl.BindVertexArray(textbox.vao)

	// uniforms
	gl.Uniform2fv(textbox.finalPositionUniform, 1, &textbox.finalPosition[0])
	gl.UniformMatrix4fv(textbox.orthographicMatrixUniform, 1, false, &textbox.Menu.Font.OrthographicMatrix[0])

	// draw border - 4 * 6: four quads with six indices apiece starting at the beginning of the vbo (0)
	gl.Uniform3fv(textbox.backgroundUniform, 1, &textbox.borderBackground[0])
	gl.DrawElementsBaseVertex(gl.TRIANGLES, int32(4*6), gl.UNSIGNED_INT, nil, int32(0))

	// draw background - start drawing after skipping the border vertices (16)
	gl.Uniform3fv(textbox.backgroundUniform, 1, &textbox.textBackground[0])
	gl.DrawElementsBaseVertex(gl.TRIANGLES, int32(1*6), gl.UNSIGNED_INT, nil, int32(16))
	gl.BindVertexArray(0)

	textbox.Text.Draw()
	textbox.Cursor.Draw()
}

func (textbox *TextBox) KeyRelease(key glfw.Key, withShift bool) {
	if textbox.IsEdit {
		switch key {
		case glfw.KeyBackspace:
			textbox.Backspace()
		case glfw.KeyEscape:
			textbox.IsEdit = false
		case glfw.KeyLeft:
			textbox.MoveCursor(-1)
		case glfw.KeyRight:
			textbox.MoveCursor(+1)
		default:
			textbox.Edit(key, withShift)
		}
	}
}

func (textbox *TextBox) Edit(key glfw.Key, withShift bool) {
	if textbox.Text.HasRune(rune(key)) {
		processRune := true
		if textbox.FilterRune != nil {
			processRune = textbox.FilterRune(rune(key))
		}
		if processRune {
			var theRune rune
			if !withShift && key >= 65 && key <= 90 {
				theRune = rune(key) + 32
			} else {
				theRune = rune(key)
			}
			if textbox.Text.MaxRuneCount > 0 && len(textbox.Text.String) == textbox.Text.MaxRuneCount {
				// too long - do nothing
			} else {
				index := textbox.CursorIndex
				r := make([]rune, len(textbox.Text.String)+1)
				copy(r, []rune(textbox.Text.String))
				copy(r[index+1:], r[index:])
				r[index] = theRune

				index += 1
				textbox.CursorIndex = index
				textbox.Text.SetString(string(r))
				textbox.Text.SetPosition(textbox.Text.Position)
				textbox.Cursor.SetPosition(
					mgl32.Vec2{
						textbox.Text.Position.X() + float32(textbox.Text.CharPosition(index)),
						textbox.Text.Position.Y(),
					})
			}
		}
	}
}

func (textbox *TextBox) SetPosition(v mgl32.Vec2) {
	// transform to orthographic coordinates ranged -1 to 1 for the shader
	textbox.finalPosition[0] = v.X() / (textbox.Menu.Font.WindowWidth / 2)
	textbox.finalPosition[1] = v.Y() / (textbox.Menu.Font.WindowHeight / 2)

	// used to build shadow data and for calling SetPosition again when needed
	textbox.Position = v
	textbox.Text.SetPosition(v)
	textbox.Cursor.SetPosition(v)
}

func (textbox *TextBox) GetBoundingBox() (X1, X2 Point) {
	x, y := textbox.Position.X(), textbox.Position.Y()
	X1.X = textbox.X1.X + x
	X1.Y = textbox.X1.Y + y
	X2.X = textbox.X2.X + x
	X2.Y = textbox.X2.Y + y
	return
}

func (textbox *TextBox) Backspace() {
	index := textbox.CursorIndex
	if len(textbox.Text.String) > 0 {
		r := make([]rune, len(textbox.Text.String)-1)
		copy(r, []rune(textbox.Text.String[0:index-1]))
		copy(r[index-1:], []rune(textbox.Text.String[index:]))

		// shift our cursor back
		index -= 1
		textbox.CursorIndex = index
		textbox.Text.SetString(string(r))
		textbox.Text.SetPosition(textbox.Text.Position)
		textbox.Cursor.SetPosition(
			mgl32.Vec2{
				textbox.Text.Position.X() + float32(textbox.Text.CharPosition(index)),
				textbox.Text.Position.Y(),
			})
	}
}

func (textbox *TextBox) OrthoToScreenCoord() (X1 Point, X2 Point) {
	x1, x2 := textbox.GetBoundingBox()
	X1.X = x1.X + textbox.Menu.WindowWidth/2
	X1.Y = x1.Y + textbox.Menu.WindowHeight/2

	X2.X = x2.X + textbox.Menu.WindowWidth/2
	X2.Y = x2.Y + textbox.Menu.WindowHeight/2
	return
}

// InsidePoint returns a point nearby the center of the label
// Used to locate a screen position where clicking can be simulated
// Click on the right side in order to place the cursor to the very right of any text
func (textbox *TextBox) InsidePoint() (P Point) {
	X1, X2 := textbox.OrthoToScreenCoord()
	P.X = (X2.X - 0.1)
	P.Y = (X2.Y-X1.Y)/2 + X1.Y
	return
}

func (textbox *TextBox) ImmediateCursorDraw() {
	textbox.Cursor.RuneCount = 1
	textbox.Time = time.Now()
}

func (textbox *TextBox) MoveCursor(offset int) {
	if textbox.CursorIndex >= 0 && (textbox.CursorIndex <= len(textbox.Text.String)) {
		textbox.CursorIndex += offset
		if textbox.CursorIndex < 0 {
			textbox.CursorIndex = 0
		}
		if textbox.CursorIndex > len(textbox.Text.String) {
			textbox.CursorIndex = len(textbox.Text.String)
		}
		textbox.Cursor.SetPosition(
			mgl32.Vec2{
				textbox.Text.Position.X() + float32(textbox.Text.CharPosition(textbox.CursorIndex)),
				textbox.Text.Position.Y(),
			})
		textbox.ImmediateCursorDraw()
	}
}

func (textbox *TextBox) IsClicked(xPos, yPos float64, button MouseClick) {
	// menu rendering (and text) is positioned in orthographic projection coordinates
	// but click positions are based on window coordinates
	// we have to transform them
	X1, X2 := textbox.OrthoToScreenCoord()
	inBox := float32(xPos) > X1.X && float32(xPos) < X2.X && float32(yPos) > X1.Y && float32(yPos) < X2.Y
	if inBox {
		index, side := textbox.Text.ClickedCharacter(xPos, float64(textbox.Menu.screenPositionOffset[0]))
		if side == v41.CSRight {
			index++
		}
		// empty string
		if side == v41.CSUnknown {
			index = 0
		}
		textbox.CursorIndex = index
		textbox.ImmediateCursorDraw()

		textbox.Cursor.SetPosition(
			mgl32.Vec2{
				textbox.Text.Position.X() + float32(textbox.Text.CharPosition(index)),
				textbox.Text.Position.Y(),
			})
		textbox.IsClick = true
		if textbox.OnClick != nil {
			textbox.OnClick(textbox, xPos, yPos, button, inBox)
		}
	} else {
		textbox.IsEdit = false
	}
}

func (textbox *TextBox) IsReleased(xPos, yPos float64, button MouseClick) {
	// anything flagged as clicked now needs to decide whether to execute its logic based on inBox
	X1, X2 := textbox.OrthoToScreenCoord()
	inBox := float32(xPos) > X1.X && float32(xPos) < X2.X && float32(yPos) > X1.Y && float32(yPos) < X2.Y
	if textbox.IsClick {
		textbox.IsEdit = true
		if textbox.OnRelease != nil {
			textbox.OnRelease(textbox, xPos, yPos, button, inBox)
		}
	}
	textbox.IsClick = false
}

func (textbox *TextBox) NavigateTo() {
	if !textbox.IsEdit {
		point := textbox.InsidePoint()
		textbox.IsClicked(float64(point.X), float64(point.Y), MouseLeft)
		textbox.IsReleased(float64(point.X), float64(point.Y), MouseLeft)
	}
}

func (textbox *TextBox) NavigateAway() bool {
	if textbox.IsEdit {
		textbox.IsEdit = false
		return true
	}
	return false
}

func (textbox *TextBox) Follow() bool {
	if textbox.IsEdit {
		return true
	}
	return false
}

func (textbox *TextBox) IsNoop() bool {
	return false
}

func (textbox *TextBox) Type() FormatableType {
	return FormatableTextbox
}
