Cloud.gov Common Concourse Resource
===================================
[![Code Climate](https://codeclimate.com/github/18F/cg-common-resource/badges/gpa.svg)](https://codeclimate.com/github/18F/cg-common-resource)

Get common concourse files needed for most pipelines.


If you want to use this resource just add the following to your pipeline:

```yaml
- name: common
  type: cg-common
  source:
    bucket_name: {{private-bucket}}
    access_key_id: {{private-access-key-id}}
    secret_access_key: {{private-secret-access-key}}
    region: us-gov-west-1 # optional - defaults to us-east-1 if not provided
    secrets_file: something.yml
    secrets_passphrase: {{private-passphrase}}
    bosh_cert: bosh.pem
```

This will get you a `secrets.yml` file that is a decrypted version of the `secrets_file` and a bosh UAA certificate, named `boshCA.crt`.

You can also define more than one secrets file by using the plural form `secrets_files`:

```yaml
- name: common
  type: cg-common
  source:
    bucket_name: {{private-bucket}}
    access_key_id: {{private-access-key-id}}
    secret_access_key: {{private-secret-access-key}}
    region: us-gov-west-1 # optional - defaults to us-east-1 if not provided
    secrets_files:
    - something.yml
    - somethingelse.yml
    secrets_passphrase: {{private-passphrase}}
    bosh_cert: bosh.pem
```

**NOTE:** All files named in `secrets_files` have to be encrypted with the same passphrase.

You will then get a version of each prefixed by `decrypted-`, e.g. `decrypted-something.yml`.