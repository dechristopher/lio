import React, { FC } from "react";
import styles from "./PromotionModal.module.scss";

interface PromotionModalProps {
	open: boolean;
}

const PromotionModal: FC<PromotionModalProps> = (props) => {
	if (!props.open) {
		return null;
	}

	return (
		<div>
			<div className={styles.promoShade} />
			<div className={styles.promo}>
				<div className={`${styles.piece} piece promo queen`} />
				<div className={`${styles.piece} piece promo rook`} />
				<div className={`${styles.piece} piece promo bishop`} />
				<div className={`${styles.piece} piece promo knight`} />
			</div>
		</div>
	);
};

export default PromotionModal;
