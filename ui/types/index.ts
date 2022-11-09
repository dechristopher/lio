export * from "./enums";

export enum VariantPool {
	Bullet = "bullet",
	Blitz = "blitz",
	Rapid = "rapid",
	Hyper = "hyper",
	Ulti = "ulti",
}

type VariantTime = {
	t: number;
	i: number;
	d: number;
};

export type Variant = {
	name: string;
	html_name: string;
	group: VariantPool;
	time: VariantTime;
};

export type VariantPools = Partial<Record<keyof typeof VariantPool, Variant[]>>;