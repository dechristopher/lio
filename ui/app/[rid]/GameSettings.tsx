import { PlayerColor, VariantGroup } from "@client/proto/ws_pb";
import styles from "./GameSettings.module.scss";

export interface GameSettingsProps {
	playerColor: PlayerColor;
	variantName: string;
	variantGroup: VariantGroup;
}

export default function GameSettings(props: GameSettingsProps) {
	return (
		<div className={styles.gameSettings}>
			<div className={styles.variant}>
				{`${props.variantName} ${VariantGroup[
					props.variantGroup
				].toLowerCase()}`}
			</div>
			<div>{PlayerColor[props.playerColor]} • CASUAL</div>
		</div>
	);
}
