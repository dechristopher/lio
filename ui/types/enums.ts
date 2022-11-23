export enum SerializedColor {
	WHITE = "w",
	BLACK = "b"
}

export enum Color {
	WHITE = "white",
	BLACK = "black",
}

export function SerializedColorToString(color: SerializedColor): Color {
	if (color === SerializedColor.BLACK) {
		return Color.BLACK;
	}

	return Color.WHITE;
}