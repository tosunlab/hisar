import { withHisar } from "./hisar";
import { copyGeneratedPassword } from "./raycast/actions";
import { getConnectOptions } from "./raycast/preferences";

/** GeneratePassword copies a password from Hisar's generator with its defaults: 24 characters, digits and symbols. */
export default async function GeneratePassword(): Promise<void> {
  await copyGeneratedPassword(() => withHisar(getConnectOptions(), (hisar) => hisar.generatePassword()));
}
