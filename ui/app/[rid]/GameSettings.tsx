import {
	SerializedColor,
	SerializedColorToString,
	VariantPool,
} from "@client/types";
import styles from "./GameSettings.module.scss";

export interface GameSettingsProps {
	playerColor: SerializedColor;
	variantName: string;
	variantGroup: VariantPool;
}

export default function GameSettings(props: GameSettingsProps) {
	return (
		<div className={styles.gameSettings}>
			<div className={styles.variant}>
				{`${props.variantName} ${props.variantGroup}`}
			</div>
			<div>
				{SerializedColorToString(props.playerColor).toUpperCase()} •
				CASUAL
			</div>
		</div>
	);
}
